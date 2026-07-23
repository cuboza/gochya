package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRefreshTokenStore struct {
	pool *pgxpool.Pool
}

var _ RefreshTokenStore = (*PostgresRefreshTokenStore)(nil)

func NewPostgresRefreshTokenStore(
	pool *pgxpool.Pool,
) (*PostgresRefreshTokenStore, error) {
	if pool == nil {
		return nil, errors.New("PostgreSQL pool is required")
	}
	return &PostgresRefreshTokenStore{pool: pool}, nil
}

func (s *PostgresRefreshTokenStore) Create(
	ctx context.Context,
	record RefreshTokenRecord,
) error {
	_, err := s.pool.Exec(
		ctx,
		`INSERT INTO refresh_tokens (
		     id, family_id, player_id, device_id, token_hash,
		     issued_at, expires_at, family_expires_at
		 ) VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7, $8)`,
		record.ID,
		record.FamilyID,
		record.PlayerID,
		record.DeviceID,
		record.TokenHash[:],
		record.IssuedAt,
		record.ExpiresAt,
		record.FamilyExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("insert refresh token: %w", err)
	}
	return nil
}

func (s *PostgresRefreshTokenStore) Rotate(
	ctx context.Context,
	currentHash [32]byte,
	replacement RefreshTokenReplacement,
	now time.Time,
) (RefreshIdentity, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return RefreshIdentity{}, fmt.Errorf("begin refresh rotation: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var (
		currentID     string
		familyID      string
		playerID      string
		deviceID      *string
		expiresAt     time.Time
		familyExpires time.Time
		revokedAt     *time.Time
	)
	err = tx.QueryRow(
		ctx,
		`SELECT id::text, family_id::text, player_id::text, device_id,
		        expires_at, family_expires_at, revoked_at
		   FROM refresh_tokens
		  WHERE token_hash = $1
		  FOR UPDATE`,
		currentHash[:],
	).Scan(
		&currentID,
		&familyID,
		&playerID,
		&deviceID,
		&expiresAt,
		&familyExpires,
		&revokedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return RefreshIdentity{}, ErrRefreshTokenInvalid
	}
	if err != nil {
		return RefreshIdentity{}, fmt.Errorf("lock refresh token: %w", err)
	}
	if revokedAt != nil {
		if err := revokeReusedFamily(ctx, tx, familyID, currentID, now); err != nil {
			return RefreshIdentity{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return RefreshIdentity{}, fmt.Errorf("commit refresh-token reuse: %w", err)
		}
		return RefreshIdentity{}, ErrRefreshTokenReused
	}
	if !now.Before(expiresAt) || !now.Before(familyExpires) {
		if _, err := tx.Exec(
			ctx,
			`UPDATE refresh_tokens
			    SET revoked_at = COALESCE(revoked_at, $2)
			  WHERE family_id = $1`,
			familyID,
			now,
		); err != nil {
			return RefreshIdentity{}, fmt.Errorf("revoke expired refresh family: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return RefreshIdentity{}, fmt.Errorf("commit expired refresh family: %w", err)
		}
		return RefreshIdentity{}, ErrRefreshTokenInvalid
	}

	replacementExpiresAt := minTime(replacement.ExpiresAt, familyExpires)
	if !replacement.IssuedAt.Before(replacementExpiresAt) {
		return RefreshIdentity{}, ErrRefreshTokenInvalid
	}
	_, err = tx.Exec(
		ctx,
		`INSERT INTO refresh_tokens (
		     id, family_id, player_id, device_id, token_hash,
		     issued_at, expires_at, family_expires_at
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		replacement.ID,
		familyID,
		playerID,
		deviceID,
		replacement.TokenHash[:],
		replacement.IssuedAt,
		replacementExpiresAt,
		familyExpires,
	)
	if err != nil {
		return RefreshIdentity{}, fmt.Errorf("insert rotated refresh token: %w", err)
	}
	result, err := tx.Exec(
		ctx,
		`UPDATE refresh_tokens
		    SET revoked_at = $2, replaced_by = $3
		  WHERE id = $1 AND revoked_at IS NULL`,
		currentID,
		now,
		replacement.ID,
	)
	if err != nil {
		return RefreshIdentity{}, fmt.Errorf("retire refresh token: %w", err)
	}
	if result.RowsAffected() != 1 {
		return RefreshIdentity{}, ErrRefreshTokenReused
	}
	if err := tx.Commit(ctx); err != nil {
		return RefreshIdentity{}, fmt.Errorf("commit refresh rotation: %w", err)
	}
	identity := RefreshIdentity{
		PlayerID:        playerID,
		FamilyID:        familyID,
		ExpiresAt:       replacementExpiresAt,
		FamilyExpiresAt: familyExpires,
	}
	if deviceID != nil {
		identity.DeviceID = *deviceID
	}
	return identity, nil
}

func (s *PostgresRefreshTokenStore) RevokeFamily(
	ctx context.Context,
	hash [32]byte,
	now time.Time,
) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("begin refresh revocation: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	var familyID string
	err = tx.QueryRow(
		ctx,
		`SELECT family_id::text
		   FROM refresh_tokens
		  WHERE token_hash = $1
		  FOR UPDATE`,
		hash[:],
	).Scan(&familyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lock refresh token for revocation: %w", err)
	}
	if _, err := tx.Exec(
		ctx,
		`UPDATE refresh_tokens
		    SET revoked_at = COALESCE(revoked_at, $2)
		  WHERE family_id = $1`,
		familyID,
		now,
	); err != nil {
		return fmt.Errorf("revoke refresh-token family: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit refresh revocation: %w", err)
	}
	return nil
}

func revokeReusedFamily(
	ctx context.Context,
	tx pgx.Tx,
	familyID string,
	reusedID string,
	now time.Time,
) error {
	if _, err := tx.Exec(
		ctx,
		`UPDATE refresh_tokens
		    SET revoked_at = COALESCE(revoked_at, $2)
		  WHERE family_id = $1`,
		familyID,
		now,
	); err != nil {
		return fmt.Errorf("revoke reused refresh-token family: %w", err)
	}
	if _, err := tx.Exec(
		ctx,
		`UPDATE refresh_tokens
		    SET reuse_detected_at = COALESCE(reuse_detected_at, $2)
		  WHERE id = $1`,
		reusedID,
		now,
	); err != nil {
		return fmt.Errorf("mark refresh-token reuse: %w", err)
	}
	return nil
}
