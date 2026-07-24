package care

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gochya/gochya/server/internal/corebridge"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	maxOfflineAge = 24 * time.Hour
	maxFutureSkew = 5 * time.Minute
	sleepDuration = 8 * time.Hour
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

func (s *PostgresStore) Reconcile(
	ctx context.Context,
	input SyncCommit,
	core corebridge.CareEngine,
) (SyncResponse, error) {
	now := input.Now.UTC().Truncate(time.Second)
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return SyncResponse{}, fmt.Errorf("begin care reconciliation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockPlayer(ctx, tx, input.PlayerID); err != nil {
		return SyncResponse{}, err
	}
	state, err := lockPetState(ctx, tx, input.PlayerID, input.PetID)
	if err != nil {
		return SyncResponse{}, err
	}
	results := make([]CommandResult, 0, len(input.Commands))
	for _, command := range input.Commands {
		existing, found, conflict, err := existingOperation(
			ctx,
			tx,
			input.PlayerID,
			command,
		)
		if err != nil {
			return SyncResponse{}, err
		}
		if conflict {
			results = append(results, CommandResult{
				OperationID: command.OperationID,
				Status:      StatusRejectedInvalid,
				ErrorCode:   "idempotency_conflict",
				Snapshot:    state.snapshot(now),
			})
			continue
		}
		if found {
			if existing.Status == StatusApplied {
				existing.Status = StatusAlreadyApplied
			}
			results = append(results, existing)
			continue
		}

		if command.ClientWallTime.Before(now.Add(-maxOfflineAge)) ||
			command.ClientWallTime.After(now.Add(maxFutureSkew)) {
			result := CommandResult{
				OperationID: command.OperationID,
				Status:      StatusRejectedExpired,
				ErrorCode:   "command_expired",
				Snapshot:    state.snapshot(now),
			}
			if err := recordOperation(ctx, tx, input, command, result, now); err != nil {
				return SyncResponse{}, err
			}
			results = append(results, result)
			continue
		}
		if command.BaseRevision != state.revision {
			result := CommandResult{
				OperationID: command.OperationID,
				Status:      StatusRejectedPrecondition,
				ErrorCode:   "revision_conflict",
				Snapshot:    state.snapshot(now),
			}
			if err := recordOperation(ctx, tx, input, command, result, now); err != nil {
				return SyncResponse{}, err
			}
			results = append(results, result)
			continue
		}
		if command.Action == actionSleep &&
			state.sleepingUntil.Valid &&
			now.Before(state.sleepingUntil.Time.UTC()) {
			result := CommandResult{
				OperationID: command.OperationID,
				Status:      StatusRejectedPrecondition,
				ErrorCode:   "already_sleeping",
				Snapshot:    state.snapshot(now),
			}
			if err := recordOperation(ctx, tx, input, command, result, now); err != nil {
				return SyncResponse{}, err
			}
			results = append(results, result)
			continue
		}
		if command.ItemID != "" {
			available, err := lockCareItem(
				ctx,
				tx,
				input.PlayerID,
				command.ItemID,
			)
			if err != nil {
				return SyncResponse{}, err
			}
			if !available {
				result := CommandResult{
					OperationID: command.OperationID,
					Status:      StatusRejectedPrecondition,
					ErrorCode:   "item_unavailable",
					Snapshot:    state.snapshot(now),
				}
				if err := recordOperation(ctx, tx, input, command, result, now); err != nil {
					return SyncResponse{}, err
				}
				results = append(results, result)
				continue
			}
		}

		advanced, err := advancePetState(ctx, core, state, now)
		if err != nil {
			return SyncResponse{}, err
		}
		cared, err := core.ApplyCare(
			ctx,
			advanced.core,
			command.Action,
			command.Item,
		)
		if err != nil {
			return SyncResponse{}, fmt.Errorf("apply reconciled care: %w", err)
		}
		if err := validateCoreState(cared); err != nil {
			return SyncResponse{}, err
		}
		advanced.core = cared
		advanced.revision++
		advanced.needsUpdatedAt = now
		if command.Action == actionSleep {
			advanced.sleepingUntil = pgtype.Timestamptz{
				Time:  now.Add(sleepDuration),
				Valid: true,
			}
		} else {
			advanced.sleepingUntil = pgtype.Timestamptz{}
		}
		advanced.syncZeroSince(now)
		if command.ItemID != "" {
			if err := consumeCareItem(ctx, tx, input, command, now); err != nil {
				return SyncResponse{}, err
			}
		}
		if err := updatePetState(ctx, tx, input.PlayerID, advanced); err != nil {
			return SyncResponse{}, err
		}
		state = advanced
		result := CommandResult{
			OperationID: command.OperationID,
			Status:      StatusApplied,
			Snapshot:    state.snapshot(now),
		}
		if err := recordOperation(ctx, tx, input, command, result, now); err != nil {
			return SyncResponse{}, err
		}
		results = append(results, result)
	}

	response := SyncResponse{
		Results:            results,
		CanonicalSnapshots: []PetSnapshot{state.snapshot(now)},
		NewRevision:        state.revision,
		ServerTime:         now,
	}
	if err := tx.Commit(ctx); err != nil {
		return SyncResponse{}, fmt.Errorf("commit care reconciliation: %w", err)
	}
	return response, nil
}

type postgresPetState struct {
	id             string
	core           corebridge.NeedsState
	revision       uint64
	needsUpdatedAt time.Time
	needsZeroSince pgtype.Timestamptz
	sleepingUntil  pgtype.Timestamptz
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
		return fmt.Errorf("lock care player: %w", err)
	}
	return nil
}

func lockPetState(
	ctx context.Context,
	tx pgx.Tx,
	playerID string,
	petID string,
) (postgresPetState, error) {
	var state postgresPetState
	var needsJSON []byte
	var revision int64
	var hungerRemainder, energyRemainder, hygieneRemainder, moodRemainder int64
	err := tx.QueryRow(ctx, `SELECT id::text,needs,care_revision,
		needs_updated_at,hunger_decay_remainder,energy_decay_remainder,
		hygiene_decay_remainder,mood_decay_remainder,needs_zero_since,
		sleeping_until,is_weak
		FROM pets WHERE owner_id=$1 AND id=$2 FOR UPDATE`,
		playerID,
		petID,
	).Scan(
		&state.id,
		&needsJSON,
		&revision,
		&state.needsUpdatedAt,
		&hungerRemainder,
		&energyRemainder,
		&hygieneRemainder,
		&moodRemainder,
		&state.needsZeroSince,
		&state.sleepingUntil,
		&state.core.Weak,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return postgresPetState{}, ErrPetNotFound
	}
	if err != nil {
		return postgresPetState{}, fmt.Errorf("lock care pet: %w", err)
	}
	needs, err := decodeNeeds(needsJSON)
	if err != nil ||
		revision < 0 ||
		hungerRemainder < 0 || hungerRemainder >= 10_800_000 ||
		energyRemainder < 0 || energyRemainder >= 10_800_000 ||
		hygieneRemainder < 0 || hygieneRemainder >= 10_800_000 ||
		moodRemainder < 0 || moodRemainder >= 10_800_000 {
		return postgresPetState{}, ErrPetState
	}
	state.revision = uint64(revision)
	state.needsUpdatedAt = state.needsUpdatedAt.UTC()
	state.core.Needs = needs
	state.core.Remainders = corebridge.NeedsDecayRemainders{
		Hunger:  uint32(hungerRemainder),
		Energy:  uint32(energyRemainder),
		Hygiene: uint32(hygieneRemainder),
		Mood:    uint32(moodRemainder),
	}
	if state.needsZeroSince.Valid {
		zeroSince := state.needsZeroSince.Time.UTC()
		if zeroSince.After(state.needsUpdatedAt) {
			return postgresPetState{}, ErrPetState
		}
		state.core.ZeroStreakSeconds = uint64(
			state.needsUpdatedAt.Sub(zeroSince) / time.Second,
		)
	}
	state.core.Sleeping = state.sleepingUntil.Valid &&
		state.needsUpdatedAt.Before(state.sleepingUntil.Time.UTC())
	if err := validateCoreState(state.core); err != nil {
		return postgresPetState{}, err
	}
	return state, nil
}

func advancePetState(
	ctx context.Context,
	core corebridge.CareEngine,
	state postgresPetState,
	now time.Time,
) (postgresPetState, error) {
	cursor := state.needsUpdatedAt
	if cursor.After(now) {
		if cursor.Sub(now) > time.Second {
			return postgresPetState{}, ErrPetState
		}
		cursor = now
	}
	if state.sleepingUntil.Valid && cursor.Before(state.sleepingUntil.Time.UTC()) {
		sleepEnd := state.sleepingUntil.Time.UTC()
		if sleepEnd.After(now) {
			sleepEnd = now
		}
		state.core.Sleeping = true
		var err error
		state.core, err = advanceInChunks(ctx, core, state.core, sleepEnd.Sub(cursor))
		if err != nil {
			return postgresPetState{}, err
		}
		cursor = sleepEnd
	}
	if cursor.Before(now) {
		state.core.Sleeping = false
		var err error
		state.core, err = advanceInChunks(ctx, core, state.core, now.Sub(cursor))
		if err != nil {
			return postgresPetState{}, err
		}
	}
	state.needsUpdatedAt = now
	return state, nil
}

func advanceInChunks(
	ctx context.Context,
	core corebridge.CareEngine,
	state corebridge.NeedsState,
	elapsed time.Duration,
) (corebridge.NeedsState, error) {
	seconds := uint64(elapsed / time.Second)
	for seconds > 0 {
		chunk := min(seconds, uint64(corebridge.MaxNeedsAdvanceSeconds))
		var err error
		state, err = core.AdvanceNeeds(ctx, state, chunk)
		if err != nil {
			return corebridge.NeedsState{}, fmt.Errorf("advance reconciled needs: %w", err)
		}
		if err := validateCoreState(state); err != nil {
			return corebridge.NeedsState{}, err
		}
		seconds -= chunk
	}
	return state, nil
}

func (s *postgresPetState) syncZeroSince(now time.Time) {
	if s.core.ZeroStreakSeconds == 0 {
		s.needsZeroSince = pgtype.Timestamptz{}
		return
	}
	s.needsZeroSince = pgtype.Timestamptz{
		Time:  now.Add(-time.Duration(s.core.ZeroStreakSeconds) * time.Second),
		Valid: true,
	}
}

func (s postgresPetState) snapshot(now time.Time) PetSnapshot {
	snapshot := PetSnapshot{
		ID:             s.id,
		Needs:          s.core.Needs,
		Revision:       s.revision,
		IsWeak:         s.core.Weak,
		NeedsUpdatedAt: s.needsUpdatedAt.UTC(),
	}
	if s.needsZeroSince.Valid {
		value := s.needsZeroSince.Time.UTC()
		snapshot.NeedsZeroSince = &value
	}
	if s.sleepingUntil.Valid && now.Before(s.sleepingUntil.Time.UTC()) {
		value := s.sleepingUntil.Time.UTC()
		snapshot.SleepingUntil = &value
	}
	return snapshot
}

func updatePetState(
	ctx context.Context,
	tx pgx.Tx,
	playerID string,
	state postgresPetState,
) error {
	needsJSON, err := json.Marshal(state.core.Needs)
	if err != nil {
		return fmt.Errorf("encode reconciled needs: %w", err)
	}
	var zeroSince, sleepingUntil any
	if state.needsZeroSince.Valid {
		zeroSince = state.needsZeroSince.Time.UTC()
	}
	if state.sleepingUntil.Valid {
		sleepingUntil = state.sleepingUntil.Time.UTC()
	}
	command, err := tx.Exec(ctx, `UPDATE pets SET
		needs=$3,care_revision=$4,needs_updated_at=$5,
		hunger_decay_remainder=$6,energy_decay_remainder=$7,
		hygiene_decay_remainder=$8,mood_decay_remainder=$9,
		needs_zero_since=$10,sleeping_until=$11,is_weak=$12
		WHERE owner_id=$1 AND id=$2`,
		playerID,
		state.id,
		needsJSON,
		state.revision,
		state.needsUpdatedAt,
		state.core.Remainders.Hunger,
		state.core.Remainders.Energy,
		state.core.Remainders.Hygiene,
		state.core.Remainders.Mood,
		zeroSince,
		sleepingUntil,
		state.core.Weak,
	)
	if err != nil {
		return fmt.Errorf("update reconciled pet: %w", err)
	}
	if command.RowsAffected() != 1 {
		return ErrPetNotFound
	}
	return nil
}

func existingOperation(
	ctx context.Context,
	tx pgx.Tx,
	playerID string,
	command NormalizedCommand,
) (CommandResult, bool, bool, error) {
	var storedHash, responseJSON []byte
	err := tx.QueryRow(ctx, `SELECT request_hash,response_body
		FROM care_operations
		WHERE player_id=$1 AND operation_id=$2 FOR UPDATE`,
		playerID,
		command.OperationID,
	).Scan(&storedHash, &responseJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return CommandResult{}, false, false, nil
	}
	if err != nil {
		return CommandResult{}, false, false, fmt.Errorf(
			"query care operation: %w",
			err,
		)
	}
	if len(storedHash) != sha256.Size ||
		!bytes.Equal(storedHash, command.RequestHash[:]) {
		return CommandResult{}, false, true, nil
	}
	var result CommandResult
	if err := json.Unmarshal(responseJSON, &result); err != nil ||
		result.OperationID != command.OperationID {
		return CommandResult{}, false, false, errors.New(
			"stored care operation response is invalid",
		)
	}
	normalizeSnapshotTimes(&result.Snapshot)
	return result, true, false, nil
}

func recordOperation(
	ctx context.Context,
	tx pgx.Tx,
	input SyncCommit,
	command NormalizedCommand,
	result CommandResult,
	now time.Time,
) error {
	responseJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("encode care operation response: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO care_operations(
		player_id,operation_id,pet_id,device_id,request_hash,
		status,response_body,created_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
		input.PlayerID,
		command.OperationID,
		command.PetID,
		input.DeviceID,
		command.RequestHash[:],
		result.Status,
		responseJSON,
		now,
	); err != nil {
		return fmt.Errorf("insert care operation: %w", err)
	}
	return nil
}

func lockCareItem(
	ctx context.Context,
	tx pgx.Tx,
	playerID string,
	itemID string,
) (bool, error) {
	var quantity int
	err := tx.QueryRow(ctx, `SELECT quantity FROM player_items
		WHERE player_id=$1 AND item_id=$2 FOR UPDATE`,
		playerID,
		itemID,
	).Scan(&quantity)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lock care item %q: %w", itemID, err)
	}
	return quantity > 0, nil
}

func consumeCareItem(
	ctx context.Context,
	tx pgx.Tx,
	input SyncCommit,
	command NormalizedCommand,
	now time.Time,
) error {
	update, err := tx.Exec(ctx, `UPDATE player_items
		SET quantity=quantity-1,updated_at=$3
		WHERE player_id=$1 AND item_id=$2 AND quantity>=1`,
		input.PlayerID,
		command.ItemID,
		now,
	)
	if err != nil {
		return fmt.Errorf("consume care item %q: %w", command.ItemID, err)
	}
	if update.RowsAffected() != 1 {
		return errors.New("locked care item became unavailable")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO item_transactions(
		player_id,item_id,amount,counterparty,counterparty_amount,
		reason,ref_id,idempotency_key,created_at)
		VALUES($1,$2,-1,'system:care',1,'care',$3,'care:' || $3,$4)`,
		input.PlayerID,
		command.ItemID,
		command.OperationID,
		now,
	); err != nil {
		return fmt.Errorf("insert care item ledger %q: %w", command.ItemID, err)
	}
	return nil
}

func decodeNeeds(data []byte) (corebridge.Needs, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil ||
		len(object) != 4 ||
		object["hunger"] == nil ||
		object["energy"] == nil ||
		object["hygiene"] == nil ||
		object["mood"] == nil {
		return corebridge.Needs{}, ErrPetState
	}
	var needs corebridge.Needs
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&needs); err != nil ||
		needs.Hunger > 100 ||
		needs.Energy > 100 ||
		needs.Hygiene > 100 ||
		needs.Mood > 100 {
		return corebridge.Needs{}, ErrPetState
	}
	return needs, nil
}

func validateCoreState(state corebridge.NeedsState) error {
	if state.Needs.Hunger > 100 ||
		state.Needs.Energy > 100 ||
		state.Needs.Hygiene > 100 ||
		state.Needs.Mood > 100 ||
		state.Remainders.Hunger >= 10_800_000 ||
		state.Remainders.Energy >= 10_800_000 ||
		state.Remainders.Hygiene >= 10_800_000 ||
		state.Remainders.Mood >= 10_800_000 {
		return ErrPetState
	}
	return nil
}

func normalizeSnapshotTimes(snapshot *PetSnapshot) {
	snapshot.NeedsUpdatedAt = snapshot.NeedsUpdatedAt.UTC()
	if snapshot.NeedsZeroSince != nil {
		value := snapshot.NeedsZeroSince.UTC()
		snapshot.NeedsZeroSince = &value
	}
	if snapshot.SleepingUntil != nil {
		value := snapshot.SleepingUntil.UTC()
		snapshot.SleepingUntil = &value
	}
}
