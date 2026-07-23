package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresIdentityStore struct {
	pool *pgxpool.Pool
}

var _ IdentityStore = (*PostgresIdentityStore)(nil)

func NewPostgresIdentityStore(pool *pgxpool.Pool) (*PostgresIdentityStore, error) {
	if pool == nil {
		return nil, errors.New("PostgreSQL pool is required")
	}
	return &PostgresIdentityStore{pool: pool}, nil
}

func (s *PostgresIdentityStore) Resolve(
	ctx context.Context,
	candidate PlayerCandidate,
) (Player, error) {
	var player Player
	err := s.pool.QueryRow(
		ctx,
		`INSERT INTO players (
		     id, username, display_name, created_at, last_seen,
		     auth_method, auth_subject
		 ) VALUES ($1, $2, NULLIF($3, ''), $4, $4, $5, $6)
		 ON CONFLICT (auth_method, auth_subject)
		 DO UPDATE SET last_seen = EXCLUDED.last_seen
		 RETURNING id::text, username, COALESCE(display_name, '')`,
		candidate.ID,
		candidate.Username,
		strings.TrimSpace(candidate.DisplayName),
		candidate.Now,
		candidate.Identity.Provider,
		candidate.Identity.Subject,
	).Scan(&player.ID, &player.Username, &player.DisplayName)
	if errors.Is(err, pgx.ErrNoRows) {
		return Player{}, errors.New("identity resolution returned no player")
	}
	if err != nil {
		return Player{}, fmt.Errorf("upsert player identity: %w", err)
	}
	return player, nil
}
