package profile

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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

func (s *PostgresStore) PlayerProfile(
	ctx context.Context,
	playerID string,
) (PlayerProfile, error) {
	var (
		response      PlayerProfile
		lastSeen      pgtype.Timestamptz
		streakLastDay string
	)
	err := s.pool.QueryRow(
		ctx,
		`SELECT p.id::text,
		        p.username,
		        COALESCE(p.display_name, ''),
		        p.created_at,
		        p.last_seen,
		        COALESCE(p.timezone, ''),
		        p.streak_days,
		        COALESCE(p.streak_last_day::text, ''),
		        COALESCE(active_pet.id::text, '')
		   FROM players p
		   LEFT JOIN LATERAL (
		       SELECT id
		         FROM pets
		        WHERE owner_id = p.id AND is_active = TRUE
		        LIMIT 1
		   ) active_pet ON TRUE
		  WHERE p.id = $1`,
		playerID,
	).Scan(
		&response.ID,
		&response.Username,
		&response.DisplayName,
		&response.CreatedAt,
		&lastSeen,
		&response.Timezone,
		&response.StreakDays,
		&streakLastDay,
		&response.ActivePetID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return PlayerProfile{}, ErrPlayerNotFound
	}
	if err != nil {
		return PlayerProfile{}, fmt.Errorf("query player profile: %w", err)
	}
	response.CreatedAt = response.CreatedAt.UTC()
	if lastSeen.Valid {
		value := lastSeen.Time.UTC()
		response.LastSeen = &value
	}
	response.StreakLastDay = streakLastDay
	return response, nil
}

func (s *PostgresStore) ListPets(
	ctx context.Context,
	playerID string,
) ([]Pet, error) {
	rows, err := s.pool.Query(
		ctx,
		petSelect+`
		  WHERE owner_id = $1
		  ORDER BY is_active DESC, created_at ASC, id ASC`,
		playerID,
	)
	if err != nil {
		return nil, fmt.Errorf("query pets: %w", err)
	}
	defer rows.Close()

	pets := make([]Pet, 0)
	for rows.Next() {
		pet, err := scanPet(rows)
		if err != nil {
			return nil, err
		}
		pets = append(pets, pet)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pets: %w", err)
	}
	return pets, nil
}

func (s *PostgresStore) Pet(
	ctx context.Context,
	playerID string,
	petID string,
) (Pet, error) {
	pet, err := scanPet(s.pool.QueryRow(
		ctx,
		petSelect+`
		  WHERE owner_id = $1 AND id = $2`,
		playerID,
		petID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Pet{}, ErrPetNotFound
	}
	return pet, err
}

func (s *PostgresStore) ActivatePet(
	ctx context.Context,
	input ActivatePetCommit,
) (Pet, error) {
	now := input.Now.UTC().Truncate(time.Microsecond)
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return Pet{}, fmt.Errorf("begin pet activation: %w", err)
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
		return Pet{}, ErrPlayerNotFound
	}
	if err != nil {
		return Pet{}, fmt.Errorf("lock player for pet activation: %w", err)
	}

	var alreadyActive bool
	err = tx.QueryRow(
		ctx,
		`SELECT is_active
		   FROM pets
		  WHERE owner_id = $1 AND id = $2
		  FOR UPDATE`,
		input.PlayerID,
		input.PetID,
	).Scan(&alreadyActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return Pet{}, ErrPetNotFound
	}
	if err != nil {
		return Pet{}, fmt.Errorf("lock pet for activation: %w", err)
	}

	if !alreadyActive {
		if _, err := tx.Exec(
			ctx,
			`UPDATE pets
			    SET is_active = FALSE
			  WHERE owner_id = $1 AND is_active = TRUE`,
			input.PlayerID,
		); err != nil {
			return Pet{}, fmt.Errorf("deactivate current pet: %w", err)
		}
		command, err := tx.Exec(
			ctx,
			`UPDATE pets
			    SET is_active = TRUE
			  WHERE owner_id = $1 AND id = $2`,
			input.PlayerID,
			input.PetID,
		)
		if err != nil {
			return Pet{}, fmt.Errorf("activate pet: %w", err)
		}
		if command.RowsAffected() != 1 {
			return Pet{}, ErrPetNotFound
		}
		if _, err := tx.Exec(
			ctx,
			`UPDATE player_loadouts
			    SET pet_id = $2,
			        revision = revision + 1,
			        updated_at = $3
			  WHERE player_id = $1 AND pet_id <> $2`,
			input.PlayerID,
			input.PetID,
			now,
		); err != nil {
			return Pet{}, fmt.Errorf("update loadout active pet: %w", err)
		}
	}

	pet, err := scanPet(tx.QueryRow(
		ctx,
		petSelect+`
		  WHERE owner_id = $1 AND id = $2`,
		input.PlayerID,
		input.PetID,
	))
	if err != nil {
		return Pet{}, fmt.Errorf("query activated pet: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Pet{}, fmt.Errorf("commit pet activation: %w", err)
	}
	return pet, nil
}

const petSelect = `
		SELECT id::text,
		       owner_id::text,
		       genome,
		       COALESCE(name, ''),
		       stage,
		       level,
		       xp,
		       needs,
		       stats,
		       generation,
		       is_active,
		       created_at,
		       COALESCE(parent_a_id::text, ''),
		       COALESCE(parent_b_id::text, ''),
		       last_bred_at,
		       needs_zero_since,
		       is_weak
		  FROM pets`

type rowScanner interface {
	Scan(...any) error
}

func scanPet(row rowScanner) (Pet, error) {
	var (
		pet            Pet
		genomeJSON     []byte
		needsJSON      []byte
		statsJSON      []byte
		lastBredAt     pgtype.Timestamptz
		needsZeroSince pgtype.Timestamptz
	)
	if err := row.Scan(
		&pet.ID,
		&pet.OwnerID,
		&genomeJSON,
		&pet.Name,
		&pet.Stage,
		&pet.Level,
		&pet.XP,
		&needsJSON,
		&statsJSON,
		&pet.Generation,
		&pet.IsActive,
		&pet.CreatedAt,
		&pet.ParentAID,
		&pet.ParentBID,
		&lastBredAt,
		&needsZeroSince,
		&pet.IsWeak,
	); err != nil {
		return Pet{}, err
	}
	if err := validateJSONObject(genomeJSON); err != nil {
		return Pet{}, fmt.Errorf("decode pet %q genome: %w", pet.ID, err)
	}
	needs, err := decodeNeeds(needsJSON)
	if err != nil {
		return Pet{}, fmt.Errorf("decode pet %q needs: %w", pet.ID, err)
	}
	pet.Needs = needs
	stats, err := decodeStats(statsJSON)
	if err != nil {
		return Pet{}, fmt.Errorf("decode pet %q stats: %w", pet.ID, err)
	}
	pet.Stats = stats
	pet.Genome = append(json.RawMessage(nil), genomeJSON...)
	pet.CreatedAt = pet.CreatedAt.UTC()
	if lastBredAt.Valid {
		value := lastBredAt.Time.UTC()
		pet.LastBredAt = &value
	}
	if needsZeroSince.Valid {
		value := needsZeroSince.Time.UTC()
		pet.NeedsZeroSince = &value
	}
	return pet, nil
}

func validateJSONObject(data []byte) error {
	var object map[string]json.RawMessage
	if err := decodeStrictObject(data, &object); err != nil {
		return err
	}
	if object == nil {
		return errors.New("JSON object is required")
	}
	return nil
}

func decodeNeeds(data []byte) (Needs, error) {
	var payload struct {
		Hunger  *uint8 `json:"hunger"`
		Energy  *uint8 `json:"energy"`
		Hygiene *uint8 `json:"hygiene"`
		Mood    *uint8 `json:"mood"`
	}
	if err := decodeStrictObject(data, &payload); err != nil {
		return Needs{}, err
	}
	if payload.Hunger == nil || payload.Energy == nil ||
		payload.Hygiene == nil || payload.Mood == nil {
		return Needs{}, errors.New("all need values are required")
	}
	result := Needs{
		Hunger:  *payload.Hunger,
		Energy:  *payload.Energy,
		Hygiene: *payload.Hygiene,
		Mood:    *payload.Mood,
	}
	if result.Hunger > 100 || result.Energy > 100 ||
		result.Hygiene > 100 || result.Mood > 100 {
		return Needs{}, errors.New("need values exceed 100")
	}
	return result, nil
}

func decodeStats(data []byte) (Stats, error) {
	var payload struct {
		Strength  *uint32 `json:"str"`
		Agility   *uint32 `json:"agi"`
		Endurance *uint32 `json:"end"`
		Focus     *uint32 `json:"foc"`
	}
	if err := decodeStrictObject(data, &payload); err != nil {
		return Stats{}, err
	}
	if payload.Strength == nil || payload.Agility == nil ||
		payload.Endurance == nil || payload.Focus == nil {
		return Stats{}, errors.New("all stat values are required")
	}
	return Stats{
		Strength:  *payload.Strength,
		Agility:   *payload.Agility,
		Endurance: *payload.Endurance,
		Focus:     *payload.Focus,
	}, nil
}

func decodeStrictObject(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("multiple JSON values are not allowed")
	}
	return nil
}
