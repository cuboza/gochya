package onboarding

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

	"github.com/gochya/gochya/server/internal/breeding"
	"github.com/gochya/gochya/server/internal/corebridge"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const under13PlayerID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"

func TestPostgresOnboardingIsPrivateAtomicAndHatchable(t *testing.T) {
	databaseURL := os.Getenv("GOCHYA_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("GOCHYA_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool := onboardingPostgresPool(t, ctx, databaseURL)
	now := time.Date(2026, time.July, 24, 12, 0, 0, 0, time.UTC)
	seedOnboardingPlayers(t, ctx, pool)
	store, err := NewPostgresStore(pool)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	core := &countingStarterCore{}
	baseStarter := StarterEggCommit{
		PlayerID:       testPlayerID,
		Element:        StarterElementWater,
		ElementID:      1,
		IdempotencyKey: testKey,
		RequestHash:    [32]byte{1},
		EggID:          testEggID,
		Seed:           42,
		Now:            now,
	}
	if _, err := store.SelectStarterEgg(
		ctx,
		baseStarter,
		core,
	); !errors.Is(err, ErrAgeGateRequired) {
		t.Fatalf("starter before age gate error = %v", err)
	}

	const concurrency = 8
	ageResponses := make([]AgeGateResponse, concurrency)
	ageErrors := make([]error, concurrency)
	var group sync.WaitGroup
	for index := range ageResponses {
		group.Add(1)
		go func() {
			defer group.Done()
			ageResponses[index], ageErrors[index] = store.RecordAgeGate(
				ctx,
				AgeGateCommit{
					PlayerID:       testPlayerID,
					AgeBand:        AgeBand13Plus,
					IdempotencyKey: testKey,
					Now:            now,
				},
			)
		}()
	}
	group.Wait()
	for index := range ageResponses {
		if ageErrors[index] != nil ||
			!reflect.DeepEqual(ageResponses[index], ageResponses[0]) {
			t.Fatalf(
				"age gate %d = %#v, %v",
				index,
				ageResponses[index],
				ageErrors[index],
			)
		}
	}
	if ageResponses[0].Status != AgeStatusEligible ||
		ageResponses[0].COPPARestricted ||
		!ageResponses[0].RecordedAt.Equal(now) {
		t.Fatalf("age response = %#v", ageResponses[0])
	}
	var ageRows int
	if err := pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM onboarding_age_gate WHERE player_id=$1`,
		testPlayerID,
	).Scan(&ageRows); err != nil {
		t.Fatalf("count age gate rows: %v", err)
	}
	if ageRows != 1 {
		t.Fatalf("age gate rows = %d", ageRows)
	}
	if _, err := store.RecordAgeGate(ctx, AgeGateCommit{
		PlayerID:       testPlayerID,
		AgeBand:        AgeBandUnder13,
		IdempotencyKey: testKey,
		Now:            now,
	}); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting age retry error = %v", err)
	}
	if _, err := store.RecordAgeGate(ctx, AgeGateCommit{
		PlayerID:       testPlayerID,
		AgeBand:        AgeBandUnder13,
		IdempotencyKey: "99999999-9999-4999-8999-999999999999",
		Now:            now,
	}); !errors.Is(err, ErrAgeGateLocked) {
		t.Fatalf("changed age gate error = %v", err)
	}
	var ageColumns []string
	if err := pool.QueryRow(ctx, `SELECT array_agg(column_name ORDER BY ordinal_position)
		FROM information_schema.columns
		WHERE table_schema=current_schema() AND table_name='onboarding_age_gate'`,
	).Scan(&ageColumns); err != nil {
		t.Fatalf("inspect age gate columns: %v", err)
	}
	if expected := []string{
		"player_id",
		"age_band",
		"policy_version",
		"idempotency_key",
		"recorded_at",
	}; !reflect.DeepEqual(ageColumns, expected) {
		t.Fatalf("age gate columns = %#v, want %#v", ageColumns, expected)
	}

	starterResponses := make([]StarterEggResponse, concurrency)
	starterErrors := make([]error, concurrency)
	for index := range starterResponses {
		group.Add(1)
		go func() {
			defer group.Done()
			commit := baseStarter
			commit.EggID = fmt.Sprintf(
				"40000000-0000-4000-8000-%012d",
				index+1,
			)
			commit.Seed = uint64(100 + index)
			starterResponses[index], starterErrors[index] = store.SelectStarterEgg(
				ctx,
				commit,
				core,
			)
		}()
	}
	group.Wait()
	for index := range starterResponses {
		if starterErrors[index] != nil ||
			!reflect.DeepEqual(starterResponses[index], starterResponses[0]) {
			t.Fatalf(
				"starter %d = %#v, %v",
				index,
				starterResponses[index],
				starterErrors[index],
			)
		}
	}
	if core.calls.Load() != 1 {
		t.Fatalf("starter core calls = %d", core.calls.Load())
	}
	if starterResponses[0].Element != StarterElementWater ||
		!starterResponses[0].IncubateUntil.Equal(now.Add(starterIncubation)) {
		t.Fatalf("starter response = %#v", starterResponses[0])
	}
	var eggCount, selectionCount, nullParents, seedLength, starterApples, starterLedger int
	var origin string
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT COUNT(*) FROM eggs WHERE owner_id=$1),
		(SELECT COUNT(*) FROM onboarding_starter_selections WHERE player_id=$1),
		(SELECT COUNT(*) FROM eggs
		 WHERE owner_id=$1 AND parent_a_id IS NULL AND parent_b_id IS NULL),
		(SELECT octet_length(breeding_seed) FROM eggs WHERE owner_id=$1),
		(SELECT origin FROM eggs WHERE owner_id=$1),
		(SELECT quantity FROM player_items
		 WHERE player_id=$1 AND item_id='apple'),
		(SELECT COUNT(*) FROM item_transactions
		 WHERE player_id=$1 AND reason='starter_kit')`,
		testPlayerID,
	).Scan(
		&eggCount,
		&selectionCount,
		&nullParents,
		&seedLength,
		&origin,
		&starterApples,
		&starterLedger,
	); err != nil {
		t.Fatalf("inspect starter persistence: %v", err)
	}
	if eggCount != 1 ||
		selectionCount != 1 ||
		nullParents != 1 ||
		seedLength != 8 ||
		origin != "starter" ||
		starterApples != 3 ||
		starterLedger != 1 {
		t.Fatalf(
			"starter persistence = eggs %d selections %d parents %d seed %d origin %q apples %d ledger %d",
			eggCount,
			selectionCount,
			nullParents,
			seedLength,
			origin,
			starterApples,
			starterLedger,
		)
	}

	conflict := baseStarter
	conflict.RequestHash = [32]byte{2}
	if _, err := store.SelectStarterEgg(
		ctx,
		conflict,
		core,
	); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting starter retry error = %v", err)
	}
	conflict.IdempotencyKey = "99999999-9999-4999-8999-999999999999"
	if _, err := store.SelectStarterEgg(
		ctx,
		conflict,
		core,
	); !errors.Is(err, ErrStarterAlreadySelected) {
		t.Fatalf("second starter choice error = %v", err)
	}

	breedingStore, err := breeding.NewPostgresStore(pool)
	if err != nil {
		t.Fatalf("NewPostgresBreedingStore: %v", err)
	}
	eggs, err := breedingStore.ListEggs(ctx, testPlayerID)
	if err != nil ||
		len(eggs) != 1 ||
		eggs[0].Origin != "starter" ||
		eggs[0].ParentAID != "" ||
		eggs[0].ParentBID != "" {
		t.Fatalf("starter ListEggs = %#v, %v", eggs, err)
	}
	selectedEggID := starterResponses[0].EggID
	if _, err := breedingStore.Hatch(ctx, breeding.HatchCommit{
		PlayerID: testPlayerID,
		EggID:    selectedEggID,
		PetID:    "50000000-0000-4000-8000-000000000000",
		Now:      now.Add(4 * time.Second),
	}); !errors.Is(err, breeding.ErrEggNotReady) {
		t.Fatalf("early starter hatch error = %v", err)
	}

	pets := make([]breeding.Pet, concurrency)
	hatchErrors := make([]error, concurrency)
	for index := range pets {
		group.Add(1)
		go func() {
			defer group.Done()
			pets[index], hatchErrors[index] = breedingStore.Hatch(
				ctx,
				breeding.HatchCommit{
					PlayerID: testPlayerID,
					EggID:    selectedEggID,
					PetID: fmt.Sprintf(
						"50000000-0000-4000-8000-%012d",
						index+1,
					),
					Now: now.Add(5 * time.Second),
				},
			)
		}()
	}
	group.Wait()
	for index := range pets {
		if hatchErrors[index] != nil || !reflect.DeepEqual(pets[index], pets[0]) {
			t.Fatalf("hatch %d = %#v, %v", index, pets[index], hatchErrors[index])
		}
	}
	if pets[0].ParentAID != "" ||
		pets[0].ParentBID != "" ||
		!pets[0].IsActive ||
		pets[0].Stage != "baby" ||
		pets[0].Generation != 0 {
		t.Fatalf("hatched starter pet = %#v", pets[0])
	}
	retry := baseStarter
	retry.EggID = "77777777-7777-4777-8777-777777777777"
	response, err := store.SelectStarterEgg(ctx, retry, core)
	if err != nil || !reflect.DeepEqual(response, starterResponses[0]) {
		t.Fatalf("post-hatch starter retry = %#v, %v", response, err)
	}
	var petCount int
	if err := pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM pets WHERE owner_id=$1`,
		testPlayerID,
	).Scan(&petCount); err != nil {
		t.Fatalf("count starter pets: %v", err)
	}
	if petCount != 1 {
		t.Fatalf("starter pet count = %d", petCount)
	}

	under13, err := store.RecordAgeGate(ctx, AgeGateCommit{
		PlayerID:       under13PlayerID,
		AgeBand:        AgeBandUnder13,
		IdempotencyKey: testKey,
		Now:            now,
	})
	if err != nil ||
		under13.Status != AgeStatusParentalConsentRequired ||
		!under13.COPPARestricted {
		t.Fatalf("under-13 age gate = %#v, %v", under13, err)
	}
	under13Starter := baseStarter
	under13Starter.PlayerID = under13PlayerID
	under13Starter.EggID = "88888888-8888-4888-8888-888888888888"
	if _, err := store.SelectStarterEgg(
		ctx,
		under13Starter,
		core,
	); !errors.Is(err, ErrParentalConsentRequired) {
		t.Fatalf("under-13 starter error = %v", err)
	}
}

type countingStarterCore struct {
	calls atomic.Int32
}

func (c *countingStarterCore) GenerateStarterGenome(
	_ context.Context,
	element uint8,
	_ uint64,
) (corebridge.Genome, error) {
	c.calls.Add(1)
	return corebridge.Genome{
		Visual: corebridge.VisualGenes{
			BodyShape:  element,
			PaletteHue: 120,
			PaletteSat: 80,
			Size:       4,
		},
		Stats: corebridge.StatPotentials{
			Strength:  60,
			Agility:   60,
			Endurance: 60,
			Focus:     60,
		},
		Element:      element,
		TechAffinity: 1,
	}, nil
}

func seedOnboardingPlayers(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
) {
	t.Helper()
	if _, err := pool.Exec(ctx, `INSERT INTO players(
		id,username,auth_method,auth_subject)
		VALUES
		($1,'starter','google','starter'),
		($2,'young-starter','google','young-starter')`,
		testPlayerID,
		under13PlayerID,
	); err != nil {
		t.Fatalf("insert onboarding players: %v", err)
	}
}

func onboardingPostgresPool(
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
		t.Fatalf("generate onboarding schema: %v", err)
	}
	schema := "gochya_onboarding_" + hex.EncodeToString(randomBytes)
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		admin.Close()
		t.Fatalf("create onboarding schema: %v", err)
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
		t.Fatalf("open onboarding schema: %v", err)
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
