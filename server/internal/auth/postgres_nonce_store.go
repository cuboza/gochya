package auth

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresLoginNonceStore struct {
	pool *pgxpool.Pool
}

var _ LoginNonceStore = (*PostgresLoginNonceStore)(nil)

func NewPostgresLoginNonceStore(
	pool *pgxpool.Pool,
) (*PostgresLoginNonceStore, error) {
	if pool == nil {
		return nil, errors.New("PostgreSQL pool is required")
	}
	return &PostgresLoginNonceStore{pool: pool}, nil
}

func (s *PostgresLoginNonceStore) Create(
	ctx context.Context,
	record LoginNonceRecord,
) error {
	digest := loginNonceDigest(record.Nonce)
	var bindingDigest []byte
	if record.Binding != "" {
		value := sha256.Sum256([]byte(record.Binding))
		bindingDigest = value[:]
	}
	if _, err := s.pool.Exec(
		ctx,
		`INSERT INTO auth_login_nonces (
		     nonce_hash, provider, binding_hash, issued_at, expires_at
		 ) VALUES ($1, $2, $3, $4, $5)`,
		digest[:],
		record.Provider,
		bindingDigest,
		record.IssuedAt,
		record.ExpiresAt,
	); err != nil {
		return fmt.Errorf("insert auth login nonce: %w", err)
	}
	return nil
}

func (s *PostgresLoginNonceStore) Consume(
	ctx context.Context,
	provider string,
	nonce string,
	binding string,
	now time.Time,
) error {
	digest := loginNonceDigest(nonce)
	var bindingDigest []byte
	if binding != "" {
		value := sha256.Sum256([]byte(binding))
		bindingDigest = value[:]
	}
	result, err := s.pool.Exec(
		ctx,
		`UPDATE auth_login_nonces
		    SET used_at = $4
		  WHERE nonce_hash = $1
		    AND provider = $2
		    AND binding_hash IS NOT DISTINCT FROM $3
		    AND issued_at <= $4
		    AND expires_at > $4
		    AND used_at IS NULL`,
		digest[:],
		provider,
		bindingDigest,
		now,
	)
	if err != nil {
		return fmt.Errorf("consume auth login nonce: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrLoginNonceInvalid
	}
	return nil
}
