package dojo

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultIdempotencyTTL = 24 * time.Hour
	defaultReplayTTL      = 90 * 24 * time.Hour
)

var ErrPlayerNotFound = errors.New("player not found")

var elementProtocolIDs = map[string]uint8{
	"fire":    0,
	"water":   1,
	"earth":   2,
	"air":     3,
	"light":   4,
	"dark":    5,
	"arcane":  6,
	"steam":   7,
	"magma":   8,
	"storm":   9,
	"mud":     10,
	"smoke":   11,
	"sand":    12,
	"eclipse": 13,
	"inferno": 14,
	"prism":   15,
	"crystal": 16,
}

var _ Store = (*PostgresStore)(nil)

type PostgresStoreConfig struct {
	Pool           *pgxpool.Pool
	IdempotencyTTL time.Duration
	ReplayTTL      time.Duration
}

// PostgresStore implements Store using PostgreSQL row locks and one transaction
// for every successful Dojo submission.
type PostgresStore struct {
	pool           *pgxpool.Pool
	idempotencyTTL time.Duration
	replayTTL      time.Duration
}

func NewPostgresStore(config PostgresStoreConfig) (*PostgresStore, error) {
	if config.Pool == nil {
		return nil, errors.New("PostgreSQL pool is required")
	}
	if config.IdempotencyTTL == 0 {
		config.IdempotencyTTL = defaultIdempotencyTTL
	}
	if config.ReplayTTL == 0 {
		config.ReplayTTL = defaultReplayTTL
	}
	if config.IdempotencyTTL <= 0 || config.ReplayTTL <= 0 {
		return nil, errors.New("PostgreSQL store TTLs must be positive")
	}
	return &PostgresStore{
		pool:           config.Pool,
		idempotencyTTL: config.IdempotencyTTL,
		replayTTL:      config.ReplayTTL,
	}, nil
}

func (s *PostgresStore) Device(
	ctx context.Context,
	playerID string,
	deviceID string,
) (Device, error) {
	var publicKey []byte
	var enabled bool
	err := s.pool.QueryRow(
		ctx,
		`SELECT public_key, enabled
		   FROM registered_devices
		  WHERE player_id = $1 AND device_id = $2`,
		playerID,
		deviceID,
	).Scan(&publicKey, &enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return Device{}, ErrDeviceNotFound
	}
	if err != nil {
		return Device{}, fmt.Errorf("query registered device: %w", err)
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return Device{}, fmt.Errorf("registered device public key has %d bytes", len(publicKey))
	}
	return Device{
		ID:        deviceID,
		PlayerID:  playerID,
		PublicKey: bytes.Clone(publicKey),
		Enabled:   enabled,
	}, nil
}

func (s *PostgresStore) ActiveElement(ctx context.Context, playerID string) (uint8, error) {
	var value string
	err := s.pool.QueryRow(
		ctx,
		`SELECT genome ->> 'element'
		   FROM pets
		  WHERE owner_id = $1 AND is_active = TRUE
		  ORDER BY created_at, id
		  LIMIT 1`,
		playerID,
	).Scan(&value)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrPlayerNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("query active pet element: %w", err)
	}
	element, ok := elementProtocolID(value)
	if !ok {
		return 0, fmt.Errorf("active pet has unknown element %q", value)
	}
	return element, nil
}

func (s *PostgresStore) PutNonce(ctx context.Context, nonce NonceRecord) error {
	hash := nonceDigest(nonce.Value)
	_, err := s.pool.Exec(
		ctx,
		`INSERT INTO dojo_nonces (
		     nonce_hash, player_id, device_id, app_build, challenge, trace_id,
		     evidence_schema_version, issued_at, expires_at, used_at
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		hash[:],
		nonce.PlayerID,
		nonce.DeviceID,
		nonce.AppBuild,
		nonce.Challenge,
		nonce.TraceID,
		nonce.EvidenceSchemaVersion,
		nonce.IssuedAt,
		nonce.ExpiresAt,
		nonce.UsedAt,
	)
	if err != nil {
		return fmt.Errorf("insert Dojo nonce: %w", err)
	}
	return nil
}

func (s *PostgresStore) Nonce(ctx context.Context, value string) (NonceRecord, error) {
	hash := nonceDigest(value)
	record, err := scanNonce(s.pool.QueryRow(
		ctx,
		`SELECT challenge, trace_id::text, player_id::text, device_id, app_build,
		        evidence_schema_version, issued_at, expires_at, used_at
		   FROM dojo_nonces
		  WHERE nonce_hash = $1`,
		hash[:],
	), value)
	if errors.Is(err, pgx.ErrNoRows) {
		return NonceRecord{}, ErrNonceNotFound
	}
	if err != nil {
		return NonceRecord{}, fmt.Errorf("query Dojo nonce: %w", err)
	}
	return record, nil
}

func (s *PostgresStore) Idempotency(
	ctx context.Context,
	playerID string,
	key string,
) (SubmitResponse, [32]byte, bool, error) {
	var requestHash []byte
	var responseJSON []byte
	err := s.pool.QueryRow(
		ctx,
		`SELECT request_hash, response_body
		   FROM idempotency_results
		  WHERE player_id = $1
		    AND idempotency_key = $2
		    AND expires_at > NOW()`,
		playerID,
		key,
	).Scan(&requestHash, &responseJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return SubmitResponse{}, [32]byte{}, false, nil
	}
	if err != nil {
		return SubmitResponse{}, [32]byte{}, false, fmt.Errorf("query idempotency result: %w", err)
	}
	response, hash, err := decodeStoredResult(responseJSON, requestHash)
	if err != nil {
		return SubmitResponse{}, [32]byte{}, false, err
	}
	return response, hash, true, nil
}

func (s *PostgresStore) CommitSubmit(
	ctx context.Context,
	input CommitRequest,
) (SubmitResponse, error) {
	responseJSON, err := json.Marshal(input.Response)
	if err != nil {
		return SubmitResponse{}, fmt.Errorf("encode idempotency response: %w", err)
	}
	cardJSON, err := json.Marshal(input.Response.Card)
	if err != nil {
		return SubmitResponse{}, fmt.Errorf("encode technique card: %w", err)
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return SubmitResponse{}, fmt.Errorf("begin Dojo submit transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var playerLock int
	err = tx.QueryRow(
		ctx,
		`SELECT 1 FROM players WHERE id = $1 FOR UPDATE`,
		input.PlayerID,
	).Scan(&playerLock)
	if errors.Is(err, pgx.ErrNoRows) {
		return SubmitResponse{}, ErrPlayerNotFound
	}
	if err != nil {
		return SubmitResponse{}, fmt.Errorf("lock player: %w", err)
	}

	if existing, found, err := lockedIdempotency(
		ctx,
		tx,
		input.PlayerID,
		input.IdempotencyKey,
		input.Now,
	); err != nil {
		return SubmitResponse{}, err
	} else if found {
		if existing.RequestHash != input.RequestHash {
			return SubmitResponse{}, ErrIdempotencyConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return SubmitResponse{}, fmt.Errorf("commit idempotent retry: %w", err)
		}
		return existing.Response, nil
	}

	nonceHash := nonceDigest(input.Nonce)
	nonce, err := scanNonce(tx.QueryRow(
		ctx,
		`SELECT challenge, trace_id::text, player_id::text, device_id, app_build,
		        evidence_schema_version, issued_at, expires_at, used_at
		   FROM dojo_nonces
		  WHERE nonce_hash = $1
		  FOR UPDATE`,
		nonceHash[:],
	), input.Nonce)
	if errors.Is(err, pgx.ErrNoRows) {
		return SubmitResponse{}, ErrNonceNotFound
	}
	if err != nil {
		return SubmitResponse{}, fmt.Errorf("lock Dojo nonce: %w", err)
	}
	if nonce.PlayerID != input.PlayerID ||
		nonce.DeviceID != input.DeviceID ||
		nonce.AppBuild != input.AppBuild ||
		nonce.TraceID != input.TraceID ||
		nonce.EvidenceSchemaVersion != input.EvidenceSchemaVersion {
		return SubmitResponse{}, ErrNonceNotFound
	}
	if !input.Now.Before(nonce.ExpiresAt) {
		return SubmitResponse{}, ErrNonceNotFound
	}
	if nonce.UsedAt != nil {
		return SubmitResponse{}, ErrNonceUsed
	}

	if err := s.checkReplay(ctx, tx, input.ReplayHash, input.Now); err != nil {
		return SubmitResponse{}, err
	}
	if err := checkSubmissionLimits(ctx, tx, input.PlayerID, input.Now); err != nil {
		return SubmitResponse{}, err
	}

	if _, err := tx.Exec(
		ctx,
		`INSERT INTO technique_cards (
		     id, owner_id, card_data, is_equipped, is_signature, created_at
		 ) VALUES ($1, $2, $3, FALSE, FALSE, $4)`,
		input.Response.Card.ID,
		input.PlayerID,
		cardJSON,
		input.Response.Card.CreatedAt,
	); err != nil {
		return SubmitResponse{}, fmt.Errorf("insert technique card: %w", err)
	}
	if _, err := tx.Exec(
		ctx,
		`INSERT INTO dojo_replay_hashes (
		     replay_hash, player_id, card_id, created_at, expires_at
		 ) VALUES ($1, $2, $3, $4, $5)`,
		input.ReplayHash[:],
		input.PlayerID,
		input.Response.Card.ID,
		input.Now,
		input.Now.Add(s.replayTTL),
	); err != nil {
		if isUniqueViolation(err) {
			return SubmitResponse{}, ErrReplayDetected
		}
		return SubmitResponse{}, fmt.Errorf("insert replay hash: %w", err)
	}
	if _, err := tx.Exec(
		ctx,
		`INSERT INTO dojo_submission_audit (
		     card_id, player_id, device_id, evidence_verdict,
		     app_build, classifier_version, trace_id, created_at
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		input.Response.Card.ID,
		input.PlayerID,
		input.DeviceID,
		input.Response.EvidenceVerdict,
		input.AppBuild,
		input.ClassifierVersion,
		input.TraceID,
		input.Now,
	); err != nil {
		return SubmitResponse{}, fmt.Errorf("insert Dojo audit row: %w", err)
	}
	command, err := tx.Exec(
		ctx,
		`UPDATE dojo_nonces
		    SET used_at = $2
		  WHERE nonce_hash = $1 AND used_at IS NULL`,
		nonceHash[:],
		input.Now,
	)
	if err != nil {
		return SubmitResponse{}, fmt.Errorf("consume Dojo nonce: %w", err)
	}
	if command.RowsAffected() != 1 {
		return SubmitResponse{}, ErrNonceUsed
	}
	if _, err := tx.Exec(
		ctx,
		`INSERT INTO idempotency_results (
		     player_id, idempotency_key, request_hash, http_status,
		     response_body, created_at, expires_at
		 ) VALUES ($1, $2, $3, 200, $4, $5, $6)`,
		input.PlayerID,
		input.IdempotencyKey,
		input.RequestHash[:],
		responseJSON,
		input.Now,
		input.Now.Add(s.idempotencyTTL),
	); err != nil {
		if isUniqueViolation(err) {
			return SubmitResponse{}, ErrIdempotencyConflict
		}
		return SubmitResponse{}, fmt.Errorf("insert idempotency result: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return SubmitResponse{}, fmt.Errorf("commit Dojo submission: %w", err)
	}
	return input.Response, nil
}

type storedResult struct {
	Response    SubmitResponse
	RequestHash [32]byte
}

func lockedIdempotency(
	ctx context.Context,
	tx pgx.Tx,
	playerID string,
	key string,
	now time.Time,
) (storedResult, bool, error) {
	var requestHash []byte
	var responseJSON []byte
	var expiresAt time.Time
	err := tx.QueryRow(
		ctx,
		`SELECT request_hash, response_body, expires_at
		   FROM idempotency_results
		  WHERE player_id = $1 AND idempotency_key = $2
		  FOR UPDATE`,
		playerID,
		key,
	).Scan(&requestHash, &responseJSON, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return storedResult{}, false, nil
	}
	if err != nil {
		return storedResult{}, false, fmt.Errorf("lock idempotency result: %w", err)
	}
	if !now.Before(expiresAt) {
		if _, err := tx.Exec(
			ctx,
			`DELETE FROM idempotency_results
			  WHERE player_id = $1 AND idempotency_key = $2`,
			playerID,
			key,
		); err != nil {
			return storedResult{}, false, fmt.Errorf("delete expired idempotency result: %w", err)
		}
		return storedResult{}, false, nil
	}
	response, hash, err := decodeStoredResult(responseJSON, requestHash)
	if err != nil {
		return storedResult{}, false, err
	}
	return storedResult{Response: response, RequestHash: hash}, true, nil
}

func (s *PostgresStore) checkReplay(
	ctx context.Context,
	tx pgx.Tx,
	hash [32]byte,
	now time.Time,
) error {
	var expiresAt time.Time
	err := tx.QueryRow(
		ctx,
		`SELECT expires_at
		   FROM dojo_replay_hashes
		  WHERE replay_hash = $1
		  FOR UPDATE`,
		hash[:],
	).Scan(&expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lock replay hash: %w", err)
	}
	if now.Before(expiresAt) {
		return ErrReplayDetected
	}
	if _, err := tx.Exec(
		ctx,
		`DELETE FROM dojo_replay_hashes WHERE replay_hash = $1`,
		hash[:],
	); err != nil {
		return fmt.Errorf("delete expired replay hash: %w", err)
	}
	return nil
}

func checkSubmissionLimits(
	ctx context.Context,
	tx pgx.Tx,
	playerID string,
	now time.Time,
) error {
	var lastSubmission time.Time
	err := tx.QueryRow(
		ctx,
		`SELECT created_at
		   FROM dojo_submission_audit
		  WHERE player_id = $1
		  ORDER BY created_at DESC
		  LIMIT 1`,
		playerID,
	).Scan(&lastSubmission)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("query last Dojo submission: %w", err)
	}
	if err == nil && now.Sub(lastSubmission) < time.Minute {
		return ErrSubmissionRate
	}

	dayStart := now.UTC().Truncate(24 * time.Hour)
	var dailyCount int
	if err := tx.QueryRow(
		ctx,
		`SELECT COUNT(*)
		   FROM dojo_submission_audit
		  WHERE player_id = $1
		    AND created_at >= $2
		    AND created_at < $3`,
		playerID,
		dayStart,
		dayStart.Add(24*time.Hour),
	).Scan(&dailyCount); err != nil {
		return fmt.Errorf("count daily Dojo submissions: %w", err)
	}
	if dailyCount >= 10 {
		return ErrDailyLimit
	}
	return nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanNonce(row rowScanner, value string) (NonceRecord, error) {
	var record NonceRecord
	var usedAt *time.Time
	err := row.Scan(
		&record.Challenge,
		&record.TraceID,
		&record.PlayerID,
		&record.DeviceID,
		&record.AppBuild,
		&record.EvidenceSchemaVersion,
		&record.IssuedAt,
		&record.ExpiresAt,
		&usedAt,
	)
	if err != nil {
		return NonceRecord{}, err
	}
	record.Value = value
	record.UsedAt = usedAt
	return record, nil
}

func decodeStoredResult(
	responseJSON []byte,
	requestHash []byte,
) (SubmitResponse, [32]byte, error) {
	if len(requestHash) != sha256.Size {
		return SubmitResponse{}, [32]byte{}, fmt.Errorf(
			"stored request hash has %d bytes",
			len(requestHash),
		)
	}
	var response SubmitResponse
	if err := json.Unmarshal(responseJSON, &response); err != nil {
		return SubmitResponse{}, [32]byte{}, fmt.Errorf(
			"decode stored idempotency response: %w",
			err,
		)
	}
	var hash [32]byte
	copy(hash[:], requestHash)
	return response, hash, nil
}

func nonceDigest(value string) [32]byte {
	return sha256.Sum256([]byte(value))
}

func isUniqueViolation(err error) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && postgresError.Code == "23505"
}

func elementProtocolID(value string) (uint8, bool) {
	element, ok := elementProtocolIDs[strings.ToLower(value)]
	return element, ok
}
