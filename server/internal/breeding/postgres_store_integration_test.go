package breeding

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gochya/gochya/server/internal/corebridge"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresBreedingAndHatchAreAtomicAndIdempotent(t *testing.T) {
	databaseURL := os.Getenv("GOCHYA_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("GOCHYA_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool := breedingPostgresPool(t, ctx, databaseURL)
	now := time.Date(2026, time.July, 24, 12, 0, 0, 0, time.UTC)
	seedBreedingData(t, ctx, pool, now)
	store, err := NewPostgresStore(pool)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	core := &countingBreedingCore{}
	commit := BreedCommit{
		PlayerID:         testPlayerID,
		ParentAID:        testParentA,
		ParentBID:        testParentB,
		MutationCatalyst: true,
		HybridCatalyst:   true,
		IdempotencyKey:   testKey,
		RequestHash:      [32]byte{1},
		EggID:            testEggID,
		Seed:             42,
		Now:              now,
	}

	const concurrency = 8
	responses := make([]BreedResponse, concurrency)
	failures := make([]error, concurrency)
	var group sync.WaitGroup
	for index := range responses {
		group.Add(1)
		go func() {
			defer group.Done()
			responses[index], failures[index] = store.Breed(ctx, commit, core)
		}()
	}
	group.Wait()
	for index := range responses {
		if failures[index] != nil ||
			!reflect.DeepEqual(responses[index], responses[0]) {
			t.Fatalf(
				"breed %d = %#v, %v",
				index,
				responses[index],
				failures[index],
			)
		}
	}
	if responses[0].EggID != testEggID ||
		!responses[0].IncubateUntil.Equal(now.Add(4*time.Hour)) {
		t.Fatalf("breed response = %#v", responses[0])
	}
	if core.calls.Load() != 1 {
		t.Fatalf("core calls = %d", core.calls.Load())
	}
	assertBreedingEconomy(t, ctx, pool)

	conflict := commit
	conflict.RequestHash = [32]byte{2}
	if _, err := store.Breed(ctx, conflict, core); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting retry error = %v", err)
	}
	eggs, err := store.ListEggs(ctx, testPlayerID)
	if err != nil || len(eggs) != 1 ||
		eggs[0].ID != testEggID ||
		eggs[0].Origin != "breeding" ||
		eggs[0].Genome.Generation != 1 ||
		eggs[0].Genome.Element != 7 ||
		eggs[0].MutatedGenes != 1 {
		t.Fatalf("ListEggs = %#v, %v", eggs, err)
	}
	if _, err := store.Hatch(ctx, HatchCommit{
		PlayerID: testPlayerID,
		EggID:    testEggID,
		PetID:    "60000000-0000-4000-8000-000000000000",
		Now:      now.Add(3 * time.Hour),
	}); !errors.Is(err, ErrEggNotReady) {
		t.Fatalf("early hatch error = %v", err)
	}

	pets := make([]Pet, concurrency)
	hatchFailures := make([]error, concurrency)
	for index := range pets {
		group.Add(1)
		go func() {
			defer group.Done()
			pets[index], hatchFailures[index] = store.Hatch(ctx, HatchCommit{
				PlayerID: testPlayerID,
				EggID:    testEggID,
				PetID: fmt.Sprintf(
					"60000000-0000-4000-8000-%012d",
					index+1,
				),
				Now: now.Add(5 * time.Hour),
			})
		}()
	}
	group.Wait()
	for index := range pets {
		if hatchFailures[index] != nil ||
			!reflect.DeepEqual(pets[index], pets[0]) {
			t.Fatalf(
				"hatch %d = %#v, %v",
				index,
				pets[index],
				hatchFailures[index],
			)
		}
	}
	if pets[0].Stage != "baby" ||
		pets[0].Generation != 1 ||
		pets[0].ParentAID != testParentA ||
		pets[0].ParentBID != testParentB ||
		pets[0].IsActive ||
		pets[0].Needs != (Needs{Hunger: 100, Energy: 100, Hygiene: 100, Mood: 100}) ||
		pets[0].Stats != (Stats{Strength: 1, Agility: 1, Endurance: 1, Focus: 1}) {
		t.Fatalf("hatched pet = %#v", pets[0])
	}
	var petCount, hatchedEggs int
	if err := pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM pets WHERE owner_id=$1`,
		testPlayerID,
	).Scan(&petCount); err != nil {
		t.Fatalf("count pets: %v", err)
	}
	if err := pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM eggs
		  WHERE owner_id=$1 AND hatched_pet_id=$2`,
		testPlayerID,
		pets[0].ID,
	).Scan(&hatchedEggs); err != nil {
		t.Fatalf("count hatched eggs: %v", err)
	}
	if petCount != 3 || hatchedEggs != 1 {
		t.Fatalf("pets/hatched eggs = %d/%d", petCount, hatchedEggs)
	}
	eggs, err = store.ListEggs(ctx, testPlayerID)
	if err != nil || eggs == nil || len(eggs) != 0 {
		t.Fatalf("post-hatch eggs = %#v, %v", eggs, err)
	}
}

func TestPostgresBreedingRejectsIneligibleRequestsWithoutCosts(t *testing.T) {
	databaseURL := os.Getenv("GOCHYA_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("GOCHYA_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool := breedingPostgresPool(t, ctx, databaseURL)
	now := time.Date(2026, time.July, 24, 12, 0, 0, 0, time.UTC)
	seedBreedingData(t, ctx, pool, now)
	store, err := NewPostgresStore(pool)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	core := &countingBreedingCore{}
	commit := BreedCommit{
		PlayerID:       testPlayerID,
		ParentAID:      testParentA,
		ParentBID:      testParentB,
		IdempotencyKey: testKey,
		RequestHash:    [32]byte{1},
		EggID:          testEggID,
		Seed:           42,
		Now:            now,
	}

	if _, err := pool.Exec(
		ctx,
		`UPDATE pets SET stage='baby' WHERE id=$1`,
		testParentA,
	); err != nil {
		t.Fatalf("make parent underage: %v", err)
	}
	assertBreedError(t, ctx, store, commit, core, ErrParentStateInvalid)
	if _, err := pool.Exec(
		ctx,
		`UPDATE pets SET stage='adult',last_bred_at=$2 WHERE id=$1`,
		testParentA,
		now,
	); err != nil {
		t.Fatalf("set parent cooldown: %v", err)
	}
	assertBreedError(t, ctx, store, commit, core, ErrParentCooldown)
	if _, err := pool.Exec(
		ctx,
		`UPDATE pets SET last_bred_at=NULL WHERE id=$1`,
		testParentA,
	); err != nil {
		t.Fatalf("clear parent cooldown: %v", err)
	}
	if _, err := pool.Exec(
		ctx,
		`UPDATE player_wallet SET koins=499 WHERE player_id=$1`,
		testPlayerID,
	); err != nil {
		t.Fatalf("set insufficient wallet: %v", err)
	}
	assertBreedError(t, ctx, store, commit, core, ErrInsufficientKoins)
	if _, err := pool.Exec(
		ctx,
		`UPDATE player_wallet SET koins=1500 WHERE player_id=$1`,
		testPlayerID,
	); err != nil {
		t.Fatalf("restore breeding wallet: %v", err)
	}
	if _, err := pool.Exec(
		ctx,
		`UPDATE player_items SET quantity=0
		  WHERE player_id=$1 AND item_id=$2`,
		testPlayerID,
		ItemLoveCrystal,
	); err != nil {
		t.Fatalf("remove Love Crystal: %v", err)
	}
	assertBreedError(t, ctx, store, commit, core, ErrLoveCrystalRequired)
	if _, err := pool.Exec(
		ctx,
		`UPDATE player_items SET quantity=2
		  WHERE player_id=$1 AND item_id=$2`,
		testPlayerID,
		ItemLoveCrystal,
	); err != nil {
		t.Fatalf("restore Love Crystal: %v", err)
	}
	if _, err := pool.Exec(
		ctx,
		`UPDATE player_items SET quantity=0
		  WHERE player_id=$1 AND item_id=$2`,
		testPlayerID,
		ItemMutationCatalyst,
	); err != nil {
		t.Fatalf("remove mutation catalyst: %v", err)
	}
	withMutation := commit
	withMutation.MutationCatalyst = true
	assertBreedError(t, ctx, store, withMutation, core, ErrCatalystRequired)
	if _, err := pool.Exec(
		ctx,
		`UPDATE player_items SET quantity=1
		  WHERE player_id=$1 AND item_id=$2`,
		testPlayerID,
		ItemMutationCatalyst,
	); err != nil {
		t.Fatalf("restore mutation catalyst: %v", err)
	}
	seedSharedLineage(t, ctx, pool, now)
	assertBreedError(t, ctx, store, commit, core, ErrParentsTooRelated)

	if core.calls.Load() != 0 {
		t.Fatalf("core calls for rejected requests = %d", core.calls.Load())
	}
	var eggs, coinEntries, itemEntries int
	var koins int64
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT COUNT(*) FROM eggs WHERE owner_id=$1),
		(SELECT COUNT(*) FROM transactions WHERE player_id=$1),
		(SELECT COUNT(*) FROM item_transactions WHERE player_id=$1),
		(SELECT koins FROM player_wallet WHERE player_id=$1)`,
		testPlayerID,
	).Scan(&eggs, &coinEntries, &itemEntries, &koins); err != nil {
		t.Fatalf("query rejected breeding state: %v", err)
	}
	if eggs != 0 || coinEntries != 0 || itemEntries != 0 || koins != 1_500 {
		t.Fatalf(
			"rejected state eggs/coin tx/item tx/koins = %d/%d/%d/%d",
			eggs,
			coinEntries,
			itemEntries,
			koins,
		)
	}
}

func assertBreedError(
	t *testing.T,
	ctx context.Context,
	store *PostgresStore,
	commit BreedCommit,
	core corebridge.BreedingEngine,
	want error,
) {
	t.Helper()
	if _, err := store.Breed(ctx, commit, core); !errors.Is(err, want) {
		t.Fatalf("Breed error = %v, want %v", err, want)
	}
}

func seedSharedLineage(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	now time.Time,
) {
	t.Helper()
	genomeJSON, _ := json.Marshal(testGenome(2, 0, 3))
	ancestorIDs := []string{
		"70000000-0000-4000-8000-000000000001",
		"70000000-0000-4000-8000-000000000002",
		"70000000-0000-4000-8000-000000000003",
		"70000000-0000-4000-8000-000000000004",
	}
	for index, ancestorID := range ancestorIDs {
		if _, err := pool.Exec(ctx, `INSERT INTO pets(
			id,owner_id,genome,stage,level,xp,needs,stats,generation,
			is_active,created_at,is_weak)
			VALUES($1,$2,$3,'adult',30,0,
			'{"hunger":100,"energy":100,"hygiene":100,"mood":100}',
			'{"str":30,"agi":30,"end":30,"foc":30}',0,FALSE,$4,FALSE)`,
			ancestorID,
			testPlayerID,
			genomeJSON,
			now.Add(-time.Duration(index+1)*time.Hour),
		); err != nil {
			t.Fatalf("insert shared ancestor %d: %v", index, err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE pets
		SET parent_a_id=$3,parent_b_id=$4
		WHERE id=$1 OR id=$2`,
		testParentA,
		testParentB,
		ancestorIDs[0],
		ancestorIDs[1],
	); err != nil {
		t.Fatalf("connect direct shared lineage: %v", err)
	}
	if _, err := pool.Exec(
		ctx,
		`UPDATE pets SET parent_a_id=$2,parent_b_id=$3 WHERE id=$1`,
		ancestorIDs[0],
		ancestorIDs[2],
		ancestorIDs[3],
	); err != nil {
		t.Fatalf("connect shared lineage: %v", err)
	}
}

func assertBreedingEconomy(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
) {
	t.Helper()
	var koins, coinEntries, coinSum, coinCounterparty int64
	if err := pool.QueryRow(ctx, `SELECT koins FROM player_wallet
		WHERE player_id=$1`, testPlayerID).Scan(&koins); err != nil {
		t.Fatalf("query wallet: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*),COALESCE(SUM(amount),0),
		COALESCE(SUM(counterparty_amount),0)
		FROM transactions WHERE player_id=$1 AND reason='breed_cost'`,
		testPlayerID,
	).Scan(&coinEntries, &coinSum, &coinCounterparty); err != nil {
		t.Fatalf("query coin ledger: %v", err)
	}
	if koins != 1_000 || coinEntries != 1 ||
		coinSum != -BreedCostKoins ||
		coinCounterparty != BreedCostKoins {
		t.Fatalf(
			"wallet/coin ledger = %d/%d/%d/%d",
			koins,
			coinEntries,
			coinSum,
			coinCounterparty,
		)
	}
	rows, err := pool.Query(ctx, `SELECT item_id,quantity FROM player_items
		WHERE player_id=$1 ORDER BY item_id`, testPlayerID)
	if err != nil {
		t.Fatalf("query item inventory: %v", err)
	}
	defer rows.Close()
	quantities := map[string]int{}
	for rows.Next() {
		var item string
		var quantity int
		if err := rows.Scan(&item, &quantity); err != nil {
			t.Fatalf("scan item inventory: %v", err)
		}
		quantities[item] = quantity
	}
	if !reflect.DeepEqual(quantities, map[string]int{
		ItemLoveCrystal:      1,
		ItemMutationCatalyst: 0,
		ItemHybridCatalyst:   0,
	}) {
		t.Fatalf("item quantities = %#v", quantities)
	}
	var itemEntries, itemSum, itemCounterparty int64
	if err := pool.QueryRow(ctx, `SELECT COUNT(*),COALESCE(SUM(amount),0),
		COALESCE(SUM(counterparty_amount),0)
		FROM item_transactions WHERE player_id=$1 AND reason='breed_cost'`,
		testPlayerID,
	).Scan(&itemEntries, &itemSum, &itemCounterparty); err != nil {
		t.Fatalf("query item ledger: %v", err)
	}
	if itemEntries != 3 || itemSum != -3 || itemCounterparty != 3 {
		t.Fatalf(
			"item ledger = %d/%d/%d",
			itemEntries,
			itemSum,
			itemCounterparty,
		)
	}
	var eggs, idempotency, seedLength int
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT COUNT(*) FROM eggs WHERE owner_id=$1),
		(SELECT COUNT(*) FROM breeding_idempotency WHERE player_id=$1),
		(SELECT octet_length(breeding_seed) FROM eggs WHERE id=$2)`,
		testPlayerID,
		testEggID,
	).Scan(&eggs, &idempotency, &seedLength); err != nil {
		t.Fatalf("query breeding audit state: %v", err)
	}
	if eggs != 1 || idempotency != 1 || seedLength != 8 {
		t.Fatalf(
			"eggs/idempotency/seed length = %d/%d/%d",
			eggs,
			idempotency,
			seedLength,
		)
	}
}

type countingBreedingCore struct {
	calls atomic.Int32
}

func (c *countingBreedingCore) Breed(
	_ context.Context,
	input corebridge.BreedInput,
	_ uint64,
) (corebridge.BreedResult, error) {
	c.calls.Add(1)
	genome := input.ParentA
	genome.Element = 7
	genome.Generation = input.ParentB.Generation + 1
	return corebridge.BreedResult{
		Genome:          genome,
		IncubationHours: 4,
		MutatedGenes:    1,
	}, nil
}

func seedBreedingData(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	now time.Time,
) {
	t.Helper()
	if _, err := pool.Exec(ctx, `INSERT INTO players(
		id,username,auth_method,auth_subject)
		VALUES($1,'breeder','google','breeder')`, testPlayerID); err != nil {
		t.Fatalf("insert breeding player: %v", err)
	}
	parentA := testGenome(0, 0, 1)
	parentB := testGenome(1, 0, 2)
	parentAJSON, _ := json.Marshal(parentA)
	parentBJSON, _ := json.Marshal(parentB)
	for index, parent := range []struct {
		id     string
		genome []byte
		active bool
	}{
		{id: testParentA, genome: parentAJSON, active: true},
		{id: testParentB, genome: parentBJSON},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO pets(
			id,owner_id,genome,stage,level,xp,needs,stats,generation,
			is_active,created_at,is_weak)
			VALUES($1,$2,$3,'adult',30,0,
			'{"hunger":100,"energy":100,"hygiene":100,"mood":100}',
			'{"str":30,"agi":30,"end":30,"foc":30}',0,$4,$5,FALSE)`,
			parent.id,
			testPlayerID,
			parent.genome,
			parent.active,
			now.Add(time.Duration(index)*time.Second),
		); err != nil {
			t.Fatalf("insert breeding parent %d: %v", index, err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO player_wallet(
		player_id,koins,vitality_daily,vitality_date)
		VALUES($1,1500,0,$2)`, testPlayerID, now.Format(time.DateOnly)); err != nil {
		t.Fatalf("insert breeding wallet: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO player_items(player_id,item_id,quantity)
		VALUES($1,$2,2),($1,$3,1),($1,$4,1)`,
		testPlayerID,
		ItemLoveCrystal,
		ItemMutationCatalyst,
		ItemHybridCatalyst,
	); err != nil {
		t.Fatalf("insert breeding items: %v", err)
	}
}

func testGenome(
	element uint8,
	generation uint32,
	offset uint8,
) corebridge.Genome {
	return corebridge.Genome{
		Visual: corebridge.VisualGenes{
			BodyShape:  offset,
			PaletteHue: 30 + uint16(offset),
			PaletteSat: 60 + offset,
			Pattern:    offset,
			Size:       offset,
			EyeStyle:   offset,
			Aura:       offset,
		},
		Stats: corebridge.StatPotentials{
			Strength:  50 + offset,
			Agility:   60 + offset,
			Endurance: 70 + offset,
			Focus:     80 + offset,
		},
		Element:      element,
		TechAffinity: 1,
		Rarity:       2,
		Ability:      1,
		Generation:   generation,
	}
}

func breedingPostgresPool(
	t *testing.T,
	ctx context.Context,
	databaseURL string,
) *pgxpool.Pool {
	t.Helper()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open PostgreSQL admin pool: %v", err)
	}
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		admin.Close()
		t.Fatalf("generate breeding schema: %v", err)
	}
	schema := "gochya_breeding_" + hex.EncodeToString(randomBytes)
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		admin.Close()
		t.Fatalf("create breeding schema: %v", err)
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		admin.Close()
		t.Fatalf("parse PostgreSQL config: %v", err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		admin.Close()
		t.Fatalf("open breeding schema: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer cleanupCancel()
		_, _ = admin.Exec(cleanupCtx, "DROP SCHEMA "+identifier+" CASCADE")
		admin.Close()
	})
	migrations, err := filepath.Glob("../../migrations/*.up.sql")
	if err != nil {
		t.Fatalf("find migrations: %v", err)
	}
	sort.Strings(migrations)
	for _, migrationPath := range migrations {
		migration, err := os.ReadFile(migrationPath)
		if err != nil {
			t.Fatalf("read migration %q: %v", migrationPath, err)
		}
		if _, err := pool.Exec(ctx, string(migration)); err != nil {
			t.Fatalf("apply migration %q: %v", migrationPath, err)
		}
	}
	return pool
}
