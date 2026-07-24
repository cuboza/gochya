package onboarding

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gochya/gochya/server/internal/corebridge"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) (*PostgresStore, error) {
	if pool == nil {
		return nil, errors.New("PostgreSQL pool is required")
	}
	return &PostgresStore{pool: pool}, nil
}

func (s *PostgresStore) RecordAgeGate(
	ctx context.Context,
	input AgeGateCommit,
) (AgeGateResponse, error) {
	now := input.Now.UTC().Truncate(time.Microsecond)
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return AgeGateResponse{}, fmt.Errorf("begin age gate: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockPlayer(ctx, tx, input.PlayerID); err != nil {
		return AgeGateResponse{}, err
	}
	var storedBand, storedKey string
	var recordedAt time.Time
	err = tx.QueryRow(ctx, `SELECT age_band,idempotency_key::text,recorded_at
		FROM onboarding_age_gate WHERE player_id=$1 FOR UPDATE`,
		input.PlayerID,
	).Scan(&storedBand, &storedKey, &recordedAt)
	switch {
	case err == nil:
		if storedBand != input.AgeBand {
			if storedKey == input.IdempotencyKey {
				return AgeGateResponse{}, ErrIdempotencyConflict
			}
			return AgeGateResponse{}, ErrAgeGateLocked
		}
		response := ageGateResponse(storedBand, recordedAt)
		if err := tx.Commit(ctx); err != nil {
			return AgeGateResponse{}, fmt.Errorf("commit age gate retry: %w", err)
		}
		return response, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return AgeGateResponse{}, fmt.Errorf("query age gate: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO onboarding_age_gate(
		player_id,age_band,policy_version,idempotency_key,recorded_at)
		VALUES($1,$2,'coppa-v1',$3,$4)`,
		input.PlayerID,
		input.AgeBand,
		input.IdempotencyKey,
		now,
	); err != nil {
		return AgeGateResponse{}, fmt.Errorf("insert age gate: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return AgeGateResponse{}, fmt.Errorf("commit age gate: %w", err)
	}
	return ageGateResponse(input.AgeBand, now), nil
}

func (s *PostgresStore) SelectStarterEgg(
	ctx context.Context,
	input StarterEggCommit,
	core corebridge.StarterEngine,
) (StarterEggResponse, error) {
	now := input.Now.UTC().Truncate(time.Microsecond)
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return StarterEggResponse{}, fmt.Errorf("begin starter selection: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockPlayer(ctx, tx, input.PlayerID); err != nil {
		return StarterEggResponse{}, err
	}
	response, found, err := existingStarterResponse(ctx, tx, input)
	if err != nil {
		return StarterEggResponse{}, err
	}
	if found {
		if err := tx.Commit(ctx); err != nil {
			return StarterEggResponse{}, fmt.Errorf(
				"commit starter selection retry: %w",
				err,
			)
		}
		return response, nil
	}

	var ageBand string
	err = tx.QueryRow(ctx, `SELECT age_band FROM onboarding_age_gate
		WHERE player_id=$1 FOR UPDATE`, input.PlayerID).Scan(&ageBand)
	if errors.Is(err, pgx.ErrNoRows) {
		return StarterEggResponse{}, ErrAgeGateRequired
	}
	if err != nil {
		return StarterEggResponse{}, fmt.Errorf("query starter age gate: %w", err)
	}
	if ageBand != AgeBand13Plus {
		return StarterEggResponse{}, ErrParentalConsentRequired
	}

	var existingPets, existingEggs int
	if err := tx.QueryRow(ctx, `SELECT
		(SELECT COUNT(*) FROM pets WHERE owner_id=$1),
		(SELECT COUNT(*) FROM eggs WHERE owner_id=$1)`,
		input.PlayerID,
	).Scan(&existingPets, &existingEggs); err != nil {
		return StarterEggResponse{}, fmt.Errorf("check starter eligibility: %w", err)
	}
	if existingPets != 0 || existingEggs != 0 {
		return StarterEggResponse{}, ErrStarterUnavailable
	}

	genome, err := core.GenerateStarterGenome(ctx, input.ElementID, input.Seed)
	if err != nil {
		return StarterEggResponse{}, fmt.Errorf("generate starter genome: %w", err)
	}
	if !validStarterGenome(genome, input.ElementID) {
		return StarterEggResponse{}, ErrGenomeInvalid
	}
	genomeJSON, err := json.Marshal(genome)
	if err != nil {
		return StarterEggResponse{}, fmt.Errorf("encode starter genome: %w", err)
	}
	incubateUntil := now.Add(starterIncubation)
	seedBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(seedBytes, input.Seed)
	if _, err := tx.Exec(ctx, `INSERT INTO eggs(
		id,owner_id,origin,genome,parent_a_id,parent_b_id,incubate_until,
		breeding_seed,mutated_genes,created_at)
		VALUES($1,$2,'starter',$3,NULL,NULL,$4,$5,0,$6)`,
		input.EggID,
		input.PlayerID,
		genomeJSON,
		incubateUntil,
		seedBytes,
		now,
	); err != nil {
		return StarterEggResponse{}, fmt.Errorf("insert starter egg: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO player_items(
		player_id,item_id,quantity,updated_at)
		VALUES($1,'apple',3,$2)
		ON CONFLICT(player_id,item_id) DO UPDATE
		SET quantity=player_items.quantity+EXCLUDED.quantity,
		    updated_at=EXCLUDED.updated_at`,
		input.PlayerID,
		now,
	); err != nil {
		return StarterEggResponse{}, fmt.Errorf("grant starter apples: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO item_transactions(
		player_id,item_id,amount,counterparty,counterparty_amount,
		reason,ref_id,idempotency_key,created_at)
		VALUES($1,'apple',3,'system:onboarding',-3,
		'starter_kit',$2,'starter:' || $3,$4)`,
		input.PlayerID,
		input.EggID,
		input.IdempotencyKey,
		now,
	); err != nil {
		return StarterEggResponse{}, fmt.Errorf("insert starter item ledger: %w", err)
	}
	response = StarterEggResponse{
		EggID:         input.EggID,
		Element:       input.Element,
		IncubateUntil: incubateUntil,
	}
	responseJSON, err := json.Marshal(response)
	if err != nil {
		return StarterEggResponse{}, fmt.Errorf("encode starter response: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO onboarding_starter_selections(
		player_id,idempotency_key,request_hash,egg_id,element,response_body,created_at)
		VALUES($1,$2,$3,$4,$5,$6,$7)`,
		input.PlayerID,
		input.IdempotencyKey,
		input.RequestHash[:],
		input.EggID,
		input.Element,
		responseJSON,
		now,
	); err != nil {
		return StarterEggResponse{}, fmt.Errorf("insert starter selection: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return StarterEggResponse{}, fmt.Errorf("commit starter selection: %w", err)
	}
	return response, nil
}

func lockPlayer(ctx context.Context, tx pgx.Tx, playerID string) error {
	var lock int
	err := tx.QueryRow(
		ctx,
		`SELECT 1 FROM players WHERE id=$1 FOR UPDATE`,
		playerID,
	).Scan(&lock)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPlayerNotFound
	}
	if err != nil {
		return fmt.Errorf("lock onboarding player: %w", err)
	}
	return nil
}

func existingStarterResponse(
	ctx context.Context,
	tx pgx.Tx,
	input StarterEggCommit,
) (StarterEggResponse, bool, error) {
	var storedKey, storedEggID, storedElement string
	var storedHash, responseJSON []byte
	err := tx.QueryRow(ctx, `SELECT idempotency_key::text,request_hash,
		egg_id::text,element,response_body
		FROM onboarding_starter_selections
		WHERE player_id=$1 FOR UPDATE`,
		input.PlayerID,
	).Scan(
		&storedKey,
		&storedHash,
		&storedEggID,
		&storedElement,
		&responseJSON,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return StarterEggResponse{}, false, nil
	}
	if err != nil {
		return StarterEggResponse{}, false, fmt.Errorf(
			"query starter selection: %w",
			err,
		)
	}
	if len(storedHash) != sha256.Size ||
		!bytes.Equal(storedHash, input.RequestHash[:]) {
		if storedKey == input.IdempotencyKey {
			return StarterEggResponse{}, false, ErrIdempotencyConflict
		}
		return StarterEggResponse{}, false, ErrStarterAlreadySelected
	}
	var response StarterEggResponse
	if err := json.Unmarshal(responseJSON, &response); err != nil {
		return StarterEggResponse{}, false, fmt.Errorf(
			"decode starter selection retry: %w",
			err,
		)
	}
	if response.EggID != storedEggID ||
		response.Element != storedElement ||
		response.Element != input.Element ||
		response.IncubateUntil.IsZero() {
		return StarterEggResponse{}, false, errors.New(
			"stored starter response is inconsistent",
		)
	}
	response.IncubateUntil = response.IncubateUntil.UTC()
	return response, true, nil
}

func ageGateResponse(ageBand string, recordedAt time.Time) AgeGateResponse {
	restricted := ageBand == AgeBandUnder13
	status := AgeStatusEligible
	if restricted {
		status = AgeStatusParentalConsentRequired
	}
	return AgeGateResponse{
		Status:          status,
		COPPARestricted: restricted,
		RecordedAt:      recordedAt.UTC(),
	}
}

func validStarterGenome(genome corebridge.Genome, element uint8) bool {
	return genome.Visual.PaletteHue <= 360 &&
		genome.Visual.PaletteSat <= 100 &&
		genome.Stats.Strength <= 100 &&
		genome.Stats.Agility <= 100 &&
		genome.Stats.Endurance <= 100 &&
		genome.Stats.Focus <= 100 &&
		genome.Element == element &&
		genome.Element <= 2 &&
		genome.TechAffinity <= 5 &&
		genome.Rarity == 0 &&
		genome.Ability == 0 &&
		genome.Generation == 0
}
