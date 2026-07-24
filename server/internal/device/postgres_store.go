package device

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ Store = (*PostgresStore)(nil)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) (*PostgresStore, error) {
	if pool == nil {
		return nil, errors.New("PostgreSQL pool is required")
	}
	return &PostgresStore{pool: pool}, nil
}

func (s *PostgresStore) PutChallenge(ctx context.Context, record ChallengeRecord) error {
	hash := challengeDigest(record.Value)
	_, err := s.pool.Exec(
		ctx,
		`INSERT INTO device_enrollment_challenges (
		     challenge_hash, player_id, device_id, platform, app_build,
		     issued_at, expires_at, used_at
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		hash[:],
		record.PlayerID,
		record.DeviceID,
		record.Platform,
		record.AppBuild,
		record.IssuedAt,
		record.ExpiresAt,
		record.UsedAt,
	)
	if err != nil {
		return fmt.Errorf("insert device enrollment challenge: %w", err)
	}
	return nil
}

func (s *PostgresStore) Challenge(
	ctx context.Context,
	value string,
) (ChallengeRecord, error) {
	hash := challengeDigest(value)
	record, err := scanChallenge(s.pool.QueryRow(
		ctx,
		`SELECT player_id::text, device_id, platform, app_build,
		        issued_at, expires_at, used_at
		   FROM device_enrollment_challenges
		  WHERE challenge_hash = $1`,
		hash[:],
	), value)
	if errors.Is(err, pgx.ErrNoRows) {
		return ChallengeRecord{}, ErrChallengeNotFound
	}
	if err != nil {
		return ChallengeRecord{}, fmt.Errorf("query device enrollment challenge: %w", err)
	}
	return record, nil
}

func (s *PostgresStore) CommitRegistration(
	ctx context.Context,
	input RegistrationCommit,
) (RegisteredDevice, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return RegisteredDevice{}, fmt.Errorf("begin device registration transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	hash := challengeDigest(input.Challenge)
	challenge, err := scanChallenge(tx.QueryRow(
		ctx,
		`SELECT player_id::text, device_id, platform, app_build,
		        issued_at, expires_at, used_at
		   FROM device_enrollment_challenges
		  WHERE challenge_hash = $1
		  FOR UPDATE`,
		hash[:],
	), input.Challenge)
	if errors.Is(err, pgx.ErrNoRows) {
		return RegisteredDevice{}, ErrChallengeNotFound
	}
	if err != nil {
		return RegisteredDevice{}, fmt.Errorf("lock device enrollment challenge: %w", err)
	}
	if challenge.PlayerID != input.PlayerID ||
		challenge.DeviceID != input.DeviceID ||
		challenge.Platform != input.Platform ||
		challenge.AppBuild != input.AppBuild ||
		!input.Now.Before(challenge.ExpiresAt) {
		return RegisteredDevice{}, ErrChallengeNotFound
	}
	if challenge.UsedAt != nil {
		return RegisteredDevice{}, ErrChallengeUsed
	}

	var playerLock int
	err = tx.QueryRow(
		ctx,
		`SELECT 1 FROM players WHERE id = $1 FOR UPDATE`,
		input.PlayerID,
	).Scan(&playerLock)
	if errors.Is(err, pgx.ErrNoRows) {
		return RegisteredDevice{}, ErrPlayerNotFound
	}
	if err != nil {
		return RegisteredDevice{}, fmt.Errorf("lock player for device registration: %w", err)
	}

	var existing RegisteredDevice
	var publicKey []byte
	err = tx.QueryRow(
		ctx,
		`SELECT public_key, platform, enabled, registered_at
		   FROM registered_devices
		  WHERE player_id = $1 AND device_id = $2
		  FOR UPDATE`,
		input.PlayerID,
		input.DeviceID,
	).Scan(
		&publicKey,
		&existing.Platform,
		&existing.Enabled,
		&existing.RegisteredAt,
	)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		existing = RegisteredDevice{
			PlayerID:     input.PlayerID,
			DeviceID:     input.DeviceID,
			PublicKey:    bytes.Clone(input.PublicKey),
			Platform:     input.Platform,
			Enabled:      true,
			RegisteredAt: input.Now,
		}
		if _, err := tx.Exec(
			ctx,
			`INSERT INTO registered_devices (
			     player_id, device_id, public_key, platform, enabled, registered_at
			 ) VALUES ($1, $2, $3, $4, TRUE, $5)`,
			existing.PlayerID,
			existing.DeviceID,
			existing.PublicKey,
			existing.Platform,
			existing.RegisteredAt,
		); err != nil {
			return RegisteredDevice{}, fmt.Errorf("insert registered device: %w", err)
		}
	case err != nil:
		return RegisteredDevice{}, fmt.Errorf("lock registered device: %w", err)
	default:
		existing.PlayerID = input.PlayerID
		existing.DeviceID = input.DeviceID
		existing.PublicKey = bytes.Clone(publicKey)
		if len(publicKey) != ed25519.PublicKeySize ||
			!bytes.Equal(publicKey, input.PublicKey) ||
			existing.Platform != input.Platform ||
			!existing.Enabled {
			return RegisteredDevice{}, ErrDeviceConflict
		}
	}

	command, err := tx.Exec(
		ctx,
		`UPDATE device_enrollment_challenges
		    SET used_at = $2
		  WHERE challenge_hash = $1 AND used_at IS NULL`,
		hash[:],
		input.Now,
	)
	if err != nil {
		return RegisteredDevice{}, fmt.Errorf("consume device enrollment challenge: %w", err)
	}
	if command.RowsAffected() != 1 {
		return RegisteredDevice{}, ErrChallengeUsed
	}
	if err := tx.Commit(ctx); err != nil {
		return RegisteredDevice{}, fmt.Errorf("commit device registration: %w", err)
	}
	return existing, nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanChallenge(row rowScanner, value string) (ChallengeRecord, error) {
	var record ChallengeRecord
	err := row.Scan(
		&record.PlayerID,
		&record.DeviceID,
		&record.Platform,
		&record.AppBuild,
		&record.IssuedAt,
		&record.ExpiresAt,
		&record.UsedAt,
	)
	if err != nil {
		return ChallengeRecord{}, err
	}
	record.Value = value
	return record, nil
}

func challengeDigest(value string) [32]byte {
	return sha256.Sum256([]byte(value))
}
