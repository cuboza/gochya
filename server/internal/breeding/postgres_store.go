package breeding

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gochya/gochya/server/internal/corebridge"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const breedingCooldown = 24 * time.Hour

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) (*PostgresStore, error) {
	if pool == nil {
		return nil, errors.New("PostgreSQL pool is required")
	}
	return &PostgresStore{pool: pool}, nil
}

func (s *PostgresStore) Breed(
	ctx context.Context,
	input BreedCommit,
	core corebridge.BreedingEngine,
) (BreedResponse, error) {
	now := input.Now.UTC().Truncate(time.Microsecond)
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return BreedResponse{}, fmt.Errorf("begin breeding: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockPlayer(ctx, tx, input.PlayerID); err != nil {
		return BreedResponse{}, err
	}
	response, found, err := existingBreedResponse(ctx, tx, input)
	if err != nil {
		return BreedResponse{}, err
	}
	if found {
		if err := tx.Commit(ctx); err != nil {
			return BreedResponse{}, fmt.Errorf("commit breeding retry: %w", err)
		}
		return response, nil
	}

	parents, err := lockParents(ctx, tx, input)
	if err != nil {
		return BreedResponse{}, err
	}
	parentA, parentAFound := parents[strings.ToLower(input.ParentAID)]
	parentB, parentBFound := parents[strings.ToLower(input.ParentBID)]
	if !parentAFound || !parentBFound {
		return BreedResponse{}, ErrParentNotFound
	}
	if err := validateParentState(parentA, now); err != nil {
		return BreedResponse{}, err
	}
	if err := validateParentState(parentB, now); err != nil {
		return BreedResponse{}, err
	}
	inbreedingCoeff, err := lineageIntersection(
		ctx,
		tx,
		input.ParentAID,
		input.ParentBID,
	)
	if err != nil {
		return BreedResponse{}, err
	}
	if inbreedingCoeff > 3 {
		return BreedResponse{}, ErrParentsTooRelated
	}
	if err := validateBreedingResources(ctx, tx, input); err != nil {
		return BreedResponse{}, err
	}

	result, err := core.Breed(ctx, corebridge.BreedInput{
		ParentA:          parentA.Genome,
		ParentB:          parentB.Genome,
		MutationCatalyst: input.MutationCatalyst,
		HybridCatalyst:   input.HybridCatalyst,
		InbreedingCoeff:  uint8(inbreedingCoeff),
	}, input.Seed)
	if err != nil {
		return BreedResponse{}, fmt.Errorf("breed parent genomes: %w", err)
	}
	expectedGeneration := parentA.Genome.Generation
	if parentB.Genome.Generation > expectedGeneration {
		expectedGeneration = parentB.Genome.Generation
	}
	if expectedGeneration != ^uint32(0) {
		expectedGeneration++
	}
	if !validGenome(result.Genome) ||
		result.Genome.Generation != expectedGeneration ||
		result.IncubationHours < 4 ||
		result.IncubationHours > 24 ||
		result.MutatedGenes&^uint16(0x3fff) != 0 {
		return BreedResponse{}, ErrGenomeInvalid
	}
	genomeJSON, err := json.Marshal(result.Genome)
	if err != nil {
		return BreedResponse{}, fmt.Errorf("encode offspring genome: %w", err)
	}
	incubateUntil := now.Add(time.Duration(result.IncubationHours) * time.Hour)
	seedBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(seedBytes, input.Seed)
	if _, err := tx.Exec(ctx, `INSERT INTO eggs(
		id,owner_id,genome,parent_a_id,parent_b_id,incubate_until,
		created_at,breeding_seed,mutated_genes,origin)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'breeding')`,
		input.EggID,
		input.PlayerID,
		genomeJSON,
		input.ParentAID,
		input.ParentBID,
		incubateUntil,
		now,
		seedBytes,
		result.MutatedGenes,
	); err != nil {
		return BreedResponse{}, fmt.Errorf("insert breeding egg: %w", err)
	}
	if command, err := tx.Exec(ctx, `UPDATE pets SET last_bred_at=$3
		WHERE owner_id=$1 AND id=ANY($2::uuid[])`,
		input.PlayerID,
		[]string{input.ParentAID, input.ParentBID},
		now,
	); err != nil {
		return BreedResponse{}, fmt.Errorf("update parent cooldowns: %w", err)
	} else if command.RowsAffected() != 2 {
		return BreedResponse{}, ErrParentNotFound
	}
	if err := applyBreedingCosts(ctx, tx, input, now); err != nil {
		return BreedResponse{}, err
	}
	response = BreedResponse{
		EggID:         input.EggID,
		IncubateUntil: incubateUntil,
	}
	responseJSON, err := json.Marshal(response)
	if err != nil {
		return BreedResponse{}, fmt.Errorf("encode breeding response: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO breeding_idempotency(
		player_id,idempotency_key,request_hash,response_body,egg_id,created_at)
		VALUES($1,$2,$3,$4,$5,$6)`,
		input.PlayerID,
		input.IdempotencyKey,
		input.RequestHash[:],
		responseJSON,
		input.EggID,
		now,
	); err != nil {
		return BreedResponse{}, fmt.Errorf("insert breeding idempotency: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return BreedResponse{}, fmt.Errorf("commit breeding: %w", err)
	}
	return response, nil
}

func (s *PostgresStore) ListEggs(
	ctx context.Context,
	playerID string,
) ([]Egg, error) {
	var exists bool
	if err := s.pool.QueryRow(
		ctx,
		`SELECT EXISTS(SELECT 1 FROM players WHERE id=$1)`,
		playerID,
	).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check egg owner: %w", err)
	}
	if !exists {
		return nil, ErrPlayerNotFound
	}
	rows, err := s.pool.Query(ctx, `SELECT id::text,owner_id::text,origin,genome,
		COALESCE(parent_a_id::text,''),COALESCE(parent_b_id::text,''),
		incubate_until,mutated_genes,created_at
		FROM eggs WHERE owner_id=$1 AND hatched_at IS NULL
		ORDER BY created_at ASC,id ASC`, playerID)
	if err != nil {
		return nil, fmt.Errorf("query incubating eggs: %w", err)
	}
	defer rows.Close()

	eggs := make([]Egg, 0)
	for rows.Next() {
		var egg Egg
		var genomeJSON []byte
		if err := rows.Scan(
			&egg.ID,
			&egg.OwnerID,
			&egg.Origin,
			&genomeJSON,
			&egg.ParentAID,
			&egg.ParentBID,
			&egg.IncubateUntil,
			&egg.MutatedGenes,
			&egg.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan incubating egg: %w", err)
		}
		genome, err := decodeGenome(genomeJSON)
		if err != nil {
			return nil, ErrGenomeInvalid
		}
		egg.Genome = genome
		egg.IncubateUntil = egg.IncubateUntil.UTC()
		egg.CreatedAt = egg.CreatedAt.UTC()
		eggs = append(eggs, egg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate incubating eggs: %w", err)
	}
	return eggs, nil
}

func (s *PostgresStore) Hatch(
	ctx context.Context,
	input HatchCommit,
) (Pet, error) {
	now := input.Now.UTC().Truncate(time.Microsecond)
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Pet{}, fmt.Errorf("begin egg hatch: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockPlayer(ctx, tx, input.PlayerID); err != nil {
		return Pet{}, err
	}
	var genomeJSON []byte
	var parentAID, parentBID, hatchedPetID string
	var incubateUntil time.Time
	err = tx.QueryRow(ctx, `SELECT genome,COALESCE(parent_a_id::text,''),
		COALESCE(parent_b_id::text,''),
		incubate_until,COALESCE(hatched_pet_id::text,'')
		FROM eggs WHERE owner_id=$1 AND id=$2 FOR UPDATE`,
		input.PlayerID,
		input.EggID,
	).Scan(
		&genomeJSON,
		&parentAID,
		&parentBID,
		&incubateUntil,
		&hatchedPetID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Pet{}, ErrEggNotFound
	}
	if err != nil {
		return Pet{}, fmt.Errorf("lock egg hatch: %w", err)
	}
	if hatchedPetID != "" {
		pet, err := queryPet(ctx, tx, input.PlayerID, hatchedPetID)
		if err != nil {
			return Pet{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Pet{}, fmt.Errorf("commit repeated egg hatch: %w", err)
		}
		return pet, nil
	}
	if now.Before(incubateUntil) {
		return Pet{}, ErrEggNotReady
	}
	genome, err := decodeGenome(genomeJSON)
	if err != nil {
		return Pet{}, ErrGenomeInvalid
	}
	var activate bool
	if err := tx.QueryRow(ctx, `SELECT NOT EXISTS(
		SELECT 1 FROM pets WHERE owner_id=$1 AND is_active=TRUE)`,
		input.PlayerID,
	).Scan(&activate); err != nil {
		return Pet{}, fmt.Errorf("select hatch active state: %w", err)
	}
	needsJSON := []byte(`{"hunger":100,"energy":100,"hygiene":100,"mood":100}`)
	statsJSON := []byte(`{"str":1,"agi":1,"end":1,"foc":1}`)
	if _, err := tx.Exec(ctx, `INSERT INTO pets(
		id,owner_id,genome,stage,level,xp,needs,stats,generation,is_active,
		created_at,parent_a_id,parent_b_id,is_weak,needs_updated_at)
		VALUES($1,$2,$3,'baby',1,0,$4,$5,$6,$7,$8,$9,$10,FALSE,$8)`,
		input.PetID,
		input.PlayerID,
		genomeJSON,
		needsJSON,
		statsJSON,
		genome.Generation,
		activate,
		now,
		nullableUUID(parentAID),
		nullableUUID(parentBID),
	); err != nil {
		return Pet{}, fmt.Errorf("insert hatched pet: %w", err)
	}
	if command, err := tx.Exec(ctx, `UPDATE eggs
		SET hatched_at=$3,hatched_pet_id=$4
		WHERE owner_id=$1 AND id=$2 AND hatched_at IS NULL`,
		input.PlayerID,
		input.EggID,
		now,
		input.PetID,
	); err != nil {
		return Pet{}, fmt.Errorf("project egg hatch: %w", err)
	} else if command.RowsAffected() != 1 {
		return Pet{}, errors.New("egg hatch projection was not updated")
	}
	pet, err := queryPet(ctx, tx, input.PlayerID, input.PetID)
	if err != nil {
		return Pet{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Pet{}, fmt.Errorf("commit egg hatch: %w", err)
	}
	return pet, nil
}

type lockedParent struct {
	Genome     corebridge.Genome
	Stage      string
	Level      uint32
	LastBredAt pgtype.Timestamptz
	IsWeak     bool
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
		return fmt.Errorf("lock breeding player: %w", err)
	}
	return nil
}

func existingBreedResponse(
	ctx context.Context,
	tx pgx.Tx,
	input BreedCommit,
) (BreedResponse, bool, error) {
	var storedHash, responseJSON []byte
	err := tx.QueryRow(ctx, `SELECT request_hash,response_body
		FROM breeding_idempotency
		WHERE player_id=$1 AND idempotency_key=$2 FOR UPDATE`,
		input.PlayerID,
		input.IdempotencyKey,
	).Scan(&storedHash, &responseJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return BreedResponse{}, false, nil
	}
	if err != nil {
		return BreedResponse{}, false, fmt.Errorf("query breeding idempotency: %w", err)
	}
	if len(storedHash) != sha256.Size ||
		!bytes.Equal(storedHash, input.RequestHash[:]) {
		return BreedResponse{}, false, ErrIdempotencyConflict
	}
	var response BreedResponse
	if err := json.Unmarshal(responseJSON, &response); err != nil {
		return BreedResponse{}, false, fmt.Errorf("decode breeding retry: %w", err)
	}
	response.IncubateUntil = response.IncubateUntil.UTC()
	return response, true, nil
}

func lockParents(
	ctx context.Context,
	tx pgx.Tx,
	input BreedCommit,
) (map[string]lockedParent, error) {
	if strings.EqualFold(input.ParentAID, input.ParentBID) {
		return nil, ErrParentsIdentical
	}
	rows, err := tx.Query(ctx, `SELECT id::text,genome,stage,level,
		last_bred_at,is_weak,generation
		FROM pets WHERE owner_id=$1 AND id=ANY($2::uuid[])
		ORDER BY id FOR UPDATE`,
		input.PlayerID,
		[]string{input.ParentAID, input.ParentBID},
	)
	if err != nil {
		return nil, fmt.Errorf("lock breeding parents: %w", err)
	}
	defer rows.Close()
	parents := make(map[string]lockedParent, 2)
	for rows.Next() {
		var id, stage string
		var genomeJSON []byte
		var level, generation uint32
		var lastBredAt pgtype.Timestamptz
		var weak bool
		if err := rows.Scan(
			&id,
			&genomeJSON,
			&stage,
			&level,
			&lastBredAt,
			&weak,
			&generation,
		); err != nil {
			return nil, fmt.Errorf("scan breeding parent: %w", err)
		}
		genome, err := decodeGenome(genomeJSON)
		if err != nil || genome.Generation != generation {
			return nil, ErrGenomeInvalid
		}
		parents[strings.ToLower(id)] = lockedParent{
			Genome:     genome,
			Stage:      stage,
			Level:      level,
			LastBredAt: lastBredAt,
			IsWeak:     weak,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate breeding parents: %w", err)
	}
	if len(parents) != 2 {
		return nil, ErrParentNotFound
	}
	return parents, nil
}

func validateParentState(parent lockedParent, now time.Time) error {
	if parent.Stage != "adult" || parent.Level < 30 || parent.IsWeak {
		return ErrParentStateInvalid
	}
	if parent.LastBredAt.Valid &&
		now.Before(parent.LastBredAt.Time.UTC().Add(breedingCooldown)) {
		return ErrParentCooldown
	}
	return nil
}

func lineageIntersection(
	ctx context.Context,
	tx pgx.Tx,
	parentAID string,
	parentBID string,
) (int, error) {
	var coefficient int
	if err := tx.QueryRow(ctx, `WITH RECURSIVE lineage AS (
			SELECT id AS root_id,id AS node_id,parent_a_id,parent_b_id,0 AS depth
			FROM pets WHERE id=$1 OR id=$2
			UNION ALL
			SELECT lineage.root_id,parent.id,parent.parent_a_id,parent.parent_b_id,
			       lineage.depth+1
			FROM lineage
			JOIN pets AS parent
			  ON parent.id=lineage.parent_a_id OR parent.id=lineage.parent_b_id
			WHERE lineage.depth<3
		), distinct_lineage AS (
			SELECT DISTINCT root_id,node_id FROM lineage
		)
		SELECT COUNT(*)
		FROM distinct_lineage AS first
		JOIN distinct_lineage AS second USING(node_id)
		WHERE first.root_id=$1 AND second.root_id=$2`,
		parentAID,
		parentBID,
	).Scan(&coefficient); err != nil {
		return 0, fmt.Errorf("calculate parent relatedness: %w", err)
	}
	return coefficient, nil
}

func validateBreedingResources(
	ctx context.Context,
	tx pgx.Tx,
	input BreedCommit,
) error {
	var koins int64
	err := tx.QueryRow(ctx, `SELECT koins FROM player_wallet
		WHERE player_id=$1 FOR UPDATE`, input.PlayerID).Scan(&koins)
	if errors.Is(err, pgx.ErrNoRows) || koins < BreedCostKoins {
		return ErrInsufficientKoins
	}
	if err != nil {
		return fmt.Errorf("lock breeding wallet: %w", err)
	}
	if err := requireItem(ctx, tx, input.PlayerID, ItemLoveCrystal); err != nil {
		if errors.Is(err, ErrCatalystRequired) {
			return ErrLoveCrystalRequired
		}
		return err
	}
	if input.MutationCatalyst {
		if err := requireItem(ctx, tx, input.PlayerID, ItemMutationCatalyst); err != nil {
			return err
		}
	}
	if input.HybridCatalyst {
		if err := requireItem(ctx, tx, input.PlayerID, ItemHybridCatalyst); err != nil {
			return err
		}
	}
	return nil
}

func requireItem(
	ctx context.Context,
	tx pgx.Tx,
	playerID string,
	itemID string,
) error {
	var quantity int
	err := tx.QueryRow(ctx, `SELECT quantity FROM player_items
		WHERE player_id=$1 AND item_id=$2 FOR UPDATE`,
		playerID,
		itemID,
	).Scan(&quantity)
	if errors.Is(err, pgx.ErrNoRows) || quantity < 1 {
		return ErrCatalystRequired
	}
	if err != nil {
		return fmt.Errorf("lock breeding item %q: %w", itemID, err)
	}
	return nil
}

func applyBreedingCosts(
	ctx context.Context,
	tx pgx.Tx,
	input BreedCommit,
	now time.Time,
) error {
	if command, err := tx.Exec(ctx, `UPDATE player_wallet
		SET koins=koins-$2,updated_at=$3
		WHERE player_id=$1 AND koins>=$2`,
		input.PlayerID,
		BreedCostKoins,
		now,
	); err != nil {
		return fmt.Errorf("deduct breeding Koins: %w", err)
	} else if command.RowsAffected() != 1 {
		return ErrInsufficientKoins
	}
	if _, err := tx.Exec(ctx, `INSERT INTO transactions(
		player_id,currency,amount,counterparty,counterparty_amount,
		reason,ref_id,idempotency_key,created_at)
		VALUES($1,'koins',-$2::BIGINT,'system:breeding',$2::BIGINT,
		'breed_cost',$3,'breed:' || $4,$5)`,
		input.PlayerID,
		BreedCostKoins,
		input.EggID,
		input.IdempotencyKey,
		now,
	); err != nil {
		return fmt.Errorf("insert breeding Koins ledger entry: %w", err)
	}
	items := []string{ItemLoveCrystal}
	if input.MutationCatalyst {
		items = append(items, ItemMutationCatalyst)
	}
	if input.HybridCatalyst {
		items = append(items, ItemHybridCatalyst)
	}
	for _, itemID := range items {
		if command, err := tx.Exec(ctx, `UPDATE player_items
			SET quantity=quantity-1,updated_at=$3
			WHERE player_id=$1 AND item_id=$2 AND quantity>=1`,
			input.PlayerID,
			itemID,
			now,
		); err != nil {
			return fmt.Errorf("consume breeding item %q: %w", itemID, err)
		} else if command.RowsAffected() != 1 {
			return ErrCatalystRequired
		}
		if _, err := tx.Exec(ctx, `INSERT INTO item_transactions(
			player_id,item_id,amount,counterparty,counterparty_amount,
			reason,ref_id,idempotency_key,created_at)
			VALUES($1,$2,-1,'system:breeding',1,'breed_cost',$3,$4,$5)`,
			input.PlayerID,
			itemID,
			input.EggID,
			"breed:"+input.IdempotencyKey,
			now,
		); err != nil {
			return fmt.Errorf("insert breeding item ledger %q: %w", itemID, err)
		}
	}
	return nil
}

func queryPet(
	ctx context.Context,
	tx pgx.Tx,
	playerID string,
	petID string,
) (Pet, error) {
	var pet Pet
	var genomeJSON, needsJSON, statsJSON []byte
	err := tx.QueryRow(ctx, `SELECT id::text,owner_id::text,genome,stage,level,xp,
		needs,stats,generation,is_active,created_at,
		COALESCE(parent_a_id::text,''),COALESCE(parent_b_id::text,''),is_weak
		FROM pets WHERE owner_id=$1 AND id=$2`,
		playerID,
		petID,
	).Scan(
		&pet.ID,
		&pet.OwnerID,
		&genomeJSON,
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
		&pet.IsWeak,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Pet{}, ErrEggNotFound
	}
	if err != nil {
		return Pet{}, fmt.Errorf("query hatched pet: %w", err)
	}
	genome, err := decodeGenome(genomeJSON)
	if err != nil || genome.Generation != pet.Generation {
		return Pet{}, ErrGenomeInvalid
	}
	pet.Genome = genome
	if err := json.Unmarshal(needsJSON, &pet.Needs); err != nil {
		return Pet{}, fmt.Errorf("decode hatched pet needs: %w", err)
	}
	if err := json.Unmarshal(statsJSON, &pet.Stats); err != nil {
		return Pet{}, fmt.Errorf("decode hatched pet stats: %w", err)
	}
	pet.CreatedAt = pet.CreatedAt.UTC()
	return pet, nil
}

func nullableUUID(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func decodeGenome(data []byte) (corebridge.Genome, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil ||
		!exactKeys(
			object,
			"visual",
			"stats",
			"element",
			"techAffinity",
			"rarity",
			"ability",
			"generation",
		) {
		return corebridge.Genome{}, ErrGenomeInvalid
	}
	var visual map[string]json.RawMessage
	if err := json.Unmarshal(object["visual"], &visual); err != nil ||
		!exactKeys(
			visual,
			"bodyShape",
			"paletteHue",
			"paletteSat",
			"pattern",
			"size",
			"eyeStyle",
			"aura",
		) {
		return corebridge.Genome{}, ErrGenomeInvalid
	}
	var stats map[string]json.RawMessage
	if err := json.Unmarshal(object["stats"], &stats); err != nil ||
		!exactKeys(stats, "strPot", "agiPot", "endPot", "focPot") {
		return corebridge.Genome{}, ErrGenomeInvalid
	}
	var genome corebridge.Genome
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&genome); err != nil || !validGenome(genome) {
		return corebridge.Genome{}, ErrGenomeInvalid
	}
	return genome, nil
}

func validGenome(genome corebridge.Genome) bool {
	return genome.Visual.PaletteHue <= 360 &&
		genome.Visual.PaletteSat <= 100 &&
		genome.Stats.Strength <= 100 &&
		genome.Stats.Agility <= 100 &&
		genome.Stats.Endurance <= 100 &&
		genome.Stats.Focus <= 100 &&
		(genome.Element <= 2 || genome.Element == 7) &&
		genome.TechAffinity <= 6 &&
		genome.Rarity <= 5 &&
		genome.Ability <= 6
}

func exactKeys(object map[string]json.RawMessage, keys ...string) bool {
	if len(object) != len(keys) {
		return false
	}
	for _, key := range keys {
		if _, ok := object[key]; !ok {
			return false
		}
	}
	return true
}
