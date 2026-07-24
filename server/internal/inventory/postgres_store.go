package inventory

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gochya/gochya/server/internal/dojo"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const loadoutIdempotencyTTL = 24 * time.Hour

var _ TechniqueStore = (*PostgresStore)(nil)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) (*PostgresStore, error) {
	if pool == nil {
		return nil, errors.New("PostgreSQL pool is required")
	}
	return &PostgresStore{pool: pool}, nil
}

func (s *PostgresStore) ListTechniqueCards(
	ctx context.Context,
	playerID string,
	cursor *TechniqueCursor,
	limit int,
) ([]dojo.TechniqueCard, error) {
	var (
		rows pgx.Rows
		err  error
	)
	if cursor == nil {
		rows, err = s.pool.Query(
			ctx,
			`SELECT id::text, owner_id::text, card_data, created_at
			   FROM technique_cards
			  WHERE owner_id = $1
			  ORDER BY created_at DESC, id DESC
			  LIMIT $2`,
			playerID,
			limit,
		)
	} else {
		rows, err = s.pool.Query(
			ctx,
			`SELECT id::text, owner_id::text, card_data, created_at
			   FROM technique_cards
			  WHERE owner_id = $1
			    AND (created_at, id) < ($2, $3::uuid)
			  ORDER BY created_at DESC, id DESC
			  LIMIT $4`,
			playerID,
			cursor.CreatedAt,
			cursor.ID,
			limit,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("query technique cards: %w", err)
	}
	defer rows.Close()

	cards := make([]dojo.TechniqueCard, 0, limit)
	for rows.Next() {
		var (
			card      dojo.TechniqueCard
			id        string
			ownerID   string
			cardJSON  []byte
			createdAt time.Time
		)
		if err := rows.Scan(&id, &ownerID, &cardJSON, &createdAt); err != nil {
			return nil, fmt.Errorf("scan technique card: %w", err)
		}
		if err := json.Unmarshal(cardJSON, &card); err != nil {
			return nil, fmt.Errorf("decode technique card %q: %w", id, err)
		}
		card.ID = id
		card.OwnerID = ownerID
		card.CreatedAt = createdAt.UTC()
		cards = append(cards, card)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate technique cards: %w", err)
	}
	return cards, nil
}

func (s *PostgresStore) EquipTechniques(
	ctx context.Context,
	input EquipCommit,
) (LoadoutResponse, error) {
	now := input.Now.UTC().Truncate(time.Microsecond)
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return LoadoutResponse{}, fmt.Errorf("begin equip transaction: %w", err)
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
		return LoadoutResponse{}, ErrPlayerNotFound
	}
	if err != nil {
		return LoadoutResponse{}, fmt.Errorf("lock player for equip: %w", err)
	}

	if response, requestHash, found, err := lockedLoadoutIdempotency(
		ctx,
		tx,
		input.PlayerID,
		input.IdempotencyKey,
		now,
	); err != nil {
		return LoadoutResponse{}, err
	} else if found {
		if requestHash != input.RequestHash {
			return LoadoutResponse{}, ErrIdempotencyConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return LoadoutResponse{}, fmt.Errorf("commit idempotent equip retry: %w", err)
		}
		return response, nil
	}

	var petID string
	err = tx.QueryRow(
		ctx,
		`SELECT id::text
		   FROM pets
		  WHERE owner_id = $1 AND is_active = TRUE
		  FOR SHARE`,
		input.PlayerID,
	).Scan(&petID)
	if errors.Is(err, pgx.ErrNoRows) {
		return LoadoutResponse{}, ErrActivePetRequired
	}
	if err != nil {
		return LoadoutResponse{}, fmt.Errorf("lock active pet for equip: %w", err)
	}

	rows, err := tx.Query(
		ctx,
		`SELECT id
		   FROM technique_cards
		  WHERE owner_id = $1
		    AND id = ANY($2::uuid[])
		  FOR UPDATE`,
		input.PlayerID,
		input.CardIDs,
	)
	if err != nil {
		return LoadoutResponse{}, fmt.Errorf("lock loadout cards: %w", err)
	}
	cardCount := 0
	for rows.Next() {
		cardCount++
	}
	rowsErr := rows.Err()
	rows.Close()
	if rowsErr != nil {
		return LoadoutResponse{}, fmt.Errorf("iterate locked loadout cards: %w", rowsErr)
	}
	if cardCount != len(input.CardIDs) {
		return LoadoutResponse{}, ErrLoadoutCardsInvalid
	}

	if _, err := tx.Exec(
		ctx,
		`UPDATE technique_cards
		    SET is_equipped = FALSE,
		        is_signature = FALSE
		  WHERE owner_id = $1
		    AND (is_equipped = TRUE OR is_signature = TRUE)`,
		input.PlayerID,
	); err != nil {
		return LoadoutResponse{}, fmt.Errorf("clear equipped card projection: %w", err)
	}
	signatureID := input.CardIDs[input.SignatureIdx]
	command, err := tx.Exec(
		ctx,
		`UPDATE technique_cards
		    SET is_equipped = TRUE,
		        is_signature = (id = $3)
		  WHERE owner_id = $1
		    AND id = ANY($2::uuid[])`,
		input.PlayerID,
		input.CardIDs,
		signatureID,
	)
	if err != nil {
		return LoadoutResponse{}, fmt.Errorf("update equipped card projection: %w", err)
	}
	if command.RowsAffected() != int64(len(input.CardIDs)) {
		return LoadoutResponse{}, ErrLoadoutCardsInvalid
	}

	response := LoadoutResponse{
		PetID:        petID,
		CardIDs:      append([]string(nil), input.CardIDs...),
		SignatureIdx: input.SignatureIdx,
		UpdatedAt:    now,
	}
	if err := tx.QueryRow(
		ctx,
		`INSERT INTO player_loadouts (
		     player_id, pet_id, card_ids, signature_idx, revision, updated_at
		 ) VALUES ($1, $2, $3::uuid[], $4, 1, $5)
		 ON CONFLICT (player_id)
		 DO UPDATE SET
		     pet_id = EXCLUDED.pet_id,
		     card_ids = EXCLUDED.card_ids,
		     signature_idx = EXCLUDED.signature_idx,
		     revision = player_loadouts.revision + 1,
		     updated_at = EXCLUDED.updated_at
		 RETURNING revision`,
		input.PlayerID,
		response.PetID,
		response.CardIDs,
		response.SignatureIdx,
		response.UpdatedAt,
	).Scan(&response.Revision); err != nil {
		return LoadoutResponse{}, fmt.Errorf("upsert player loadout: %w", err)
	}
	responseJSON, err := json.Marshal(response)
	if err != nil {
		return LoadoutResponse{}, fmt.Errorf("encode equip response: %w", err)
	}
	if _, err := tx.Exec(
		ctx,
		`INSERT INTO loadout_idempotency (
		     player_id, idempotency_key, request_hash, response_body,
		     created_at, expires_at
		 ) VALUES ($1, $2, $3, $4, $5, $6)`,
		input.PlayerID,
		input.IdempotencyKey,
		input.RequestHash[:],
		responseJSON,
		now,
		now.Add(loadoutIdempotencyTTL),
	); err != nil {
		return LoadoutResponse{}, fmt.Errorf("insert loadout idempotency: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return LoadoutResponse{}, fmt.Errorf("commit equip transaction: %w", err)
	}
	return response, nil
}

func (s *PostgresStore) CurrentLoadout(
	ctx context.Context,
	playerID string,
) (LoadoutResponse, error) {
	var response LoadoutResponse
	err := s.pool.QueryRow(
		ctx,
		`SELECT pet_id::text, card_ids::text[], signature_idx, revision, updated_at
		   FROM player_loadouts
		  WHERE player_id = $1`,
		playerID,
	).Scan(
		&response.PetID,
		&response.CardIDs,
		&response.SignatureIdx,
		&response.Revision,
		&response.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return LoadoutResponse{}, ErrLoadoutNotFound
	}
	if err != nil {
		return LoadoutResponse{}, fmt.Errorf("query current loadout: %w", err)
	}
	response.UpdatedAt = response.UpdatedAt.UTC()
	return response, nil
}

func lockedLoadoutIdempotency(
	ctx context.Context,
	tx pgx.Tx,
	playerID string,
	key string,
	now time.Time,
) (LoadoutResponse, [32]byte, bool, error) {
	var (
		requestHash  []byte
		responseJSON []byte
		expiresAt    time.Time
	)
	err := tx.QueryRow(
		ctx,
		`SELECT request_hash, response_body, expires_at
		   FROM loadout_idempotency
		  WHERE player_id = $1 AND idempotency_key = $2
		  FOR UPDATE`,
		playerID,
		key,
	).Scan(&requestHash, &responseJSON, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return LoadoutResponse{}, [32]byte{}, false, nil
	}
	if err != nil {
		return LoadoutResponse{}, [32]byte{}, false, fmt.Errorf(
			"lock loadout idempotency: %w",
			err,
		)
	}
	if !now.Before(expiresAt) {
		if _, err := tx.Exec(
			ctx,
			`DELETE FROM loadout_idempotency
			  WHERE player_id = $1 AND idempotency_key = $2`,
			playerID,
			key,
		); err != nil {
			return LoadoutResponse{}, [32]byte{}, false, fmt.Errorf(
				"delete expired loadout idempotency: %w",
				err,
			)
		}
		return LoadoutResponse{}, [32]byte{}, false, nil
	}
	if len(requestHash) != sha256.Size {
		return LoadoutResponse{}, [32]byte{}, false, errors.New(
			"stored loadout request hash has invalid length",
		)
	}
	var response LoadoutResponse
	if err := json.Unmarshal(responseJSON, &response); err != nil {
		return LoadoutResponse{}, [32]byte{}, false, fmt.Errorf(
			"decode stored equip response: %w",
			err,
		)
	}
	var hash [32]byte
	copy(hash[:], requestHash)
	return response, hash, true, nil
}
