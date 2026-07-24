package profile

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	integrationOtherPlayer = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	integrationActivePet   = "22222222-2222-4222-8222-222222222222"
	integrationSecondPet   = "33333333-3333-4333-8333-333333333333"
	integrationForeignPet  = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
)

func TestPostgresProfilePetsAndAtomicActivation(t *testing.T) {
	databaseURL := os.Getenv("GOCHYA_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("GOCHYA_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool := profilePostgresTestPool(t, ctx, databaseURL)
	now := time.Date(2026, time.July, 24, 8, 30, 0, 123456000, time.UTC)
	seedProfileData(t, ctx, pool, now)

	store, err := NewPostgresStore(pool)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	service, err := NewService(ServiceConfig{
		Store: store,
		Now:   func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	player, err := service.PlayerProfile(ctx, testPlayerID)
	if err != nil {
		t.Fatalf("PlayerProfile: %v", err)
	}
	if player.ID != testPlayerID ||
		player.Username != "profile-player" ||
		player.DisplayName != "Profile Player" ||
		player.Timezone != "Asia/Tbilisi" ||
		player.StreakDays != 4 ||
		player.StreakLastDay != "2026-07-23" ||
		player.ActivePetID != integrationActivePet ||
		player.LastSeen == nil ||
		!player.LastSeen.Equal(now.Add(-time.Hour)) {
		t.Fatalf("profile = %#v", player)
	}

	pets, err := service.ListPets(ctx, testPlayerID)
	if err != nil {
		t.Fatalf("ListPets: %v", err)
	}
	if len(pets) != 2 ||
		pets[0].ID != integrationActivePet ||
		pets[1].ID != integrationSecondPet ||
		!pets[0].IsActive ||
		pets[0].Needs.Hunger != 80 ||
		pets[0].Stats.Strength != 11 ||
		pets[0].CreatedAt.Location() != time.UTC {
		t.Fatalf("pets = %#v", pets)
	}
	fetched, err := service.Pet(ctx, testPlayerID, integrationSecondPet)
	if err != nil {
		t.Fatalf("Pet: %v", err)
	}
	if fetched.ID != integrationSecondPet || fetched.IsActive {
		t.Fatalf("pet = %#v", fetched)
	}
	_, err = service.Pet(ctx, testPlayerID, integrationForeignPet)
	assertErrorCode(t, err, "pet_not_found")

	const concurrency = 8
	results := make([]Pet, concurrency)
	failures := make([]error, concurrency)
	var group sync.WaitGroup
	for index := range results {
		group.Add(1)
		go func() {
			defer group.Done()
			results[index], failures[index] = service.ActivatePet(
				ctx,
				testPlayerID,
				integrationSecondPet,
			)
		}()
	}
	group.Wait()
	for index, failure := range failures {
		if failure != nil {
			t.Fatalf("activate %d: %v", index, failure)
		}
		if !results[index].IsActive || results[index].ID != integrationSecondPet {
			t.Fatalf("activate %d result = %#v", index, results[index])
		}
	}

	var (
		activeCount  int
		activePetID  string
		loadoutPetID string
		revision     uint64
		updatedAt    time.Time
	)
	if err := pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FILTER (WHERE is_active),
		        COALESCE(MAX(id::text) FILTER (WHERE is_active), '')
		   FROM pets
		  WHERE owner_id = $1`,
		testPlayerID,
	).Scan(&activeCount, &activePetID); err != nil {
		t.Fatalf("query active pet: %v", err)
	}
	if activeCount != 1 || activePetID != integrationSecondPet {
		t.Fatalf("active count/id = %d/%q", activeCount, activePetID)
	}
	if err := pool.QueryRow(
		ctx,
		`SELECT pet_id::text, revision, updated_at
		   FROM player_loadouts
		  WHERE player_id = $1`,
		testPlayerID,
	).Scan(&loadoutPetID, &revision, &updatedAt); err != nil {
		t.Fatalf("query loadout after activation: %v", err)
	}
	if loadoutPetID != integrationSecondPet ||
		revision != 8 ||
		!updatedAt.Equal(now) {
		t.Fatalf(
			"loadout pet/revision/time = %q/%d/%s",
			loadoutPetID,
			revision,
			updatedAt,
		)
	}

	repeated, err := service.ActivatePet(ctx, testPlayerID, integrationSecondPet)
	if err != nil {
		t.Fatalf("repeat ActivatePet: %v", err)
	}
	if !reflect.DeepEqual(repeated, results[0]) {
		t.Fatalf("repeat = %#v, first = %#v", repeated, results[0])
	}
	if err := pool.QueryRow(
		ctx,
		`SELECT revision FROM player_loadouts WHERE player_id = $1`,
		testPlayerID,
	).Scan(&revision); err != nil {
		t.Fatalf("query repeated revision: %v", err)
	}
	if revision != 8 {
		t.Fatalf("revision after repeat = %d", revision)
	}

	_, err = service.ActivatePet(ctx, testPlayerID, integrationForeignPet)
	assertErrorCode(t, err, "pet_not_found")
	if err := pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FILTER (WHERE is_active),
		        COALESCE(MAX(id::text) FILTER (WHERE is_active), '')
		   FROM pets
		  WHERE owner_id = $1`,
		testPlayerID,
	).Scan(&activeCount, &activePetID); err != nil {
		t.Fatalf("query active pet after rejected activation: %v", err)
	}
	if activeCount != 1 || activePetID != integrationSecondPet {
		t.Fatalf("rejected activation changed active pet: %d/%q", activeCount, activePetID)
	}

	var indexDefinition string
	if err := pool.QueryRow(
		ctx,
		`SELECT indexdef
		   FROM pg_indexes
		  WHERE schemaname = current_schema()
		    AND indexname = 'idx_pets_owner_active_created'`,
	).Scan(&indexDefinition); err != nil {
		t.Fatalf("query pets read index: %v", err)
	}
	if !strings.Contains(indexDefinition, "created_at") ||
		!strings.Contains(indexDefinition, "id") {
		t.Fatalf("pets read index = %q", indexDefinition)
	}

	for _, invalid := range []string{
		`{}`,
		`{"hunger":101,"energy":70,"hygiene":60,"mood":90}`,
		`{"hunger":80,"energy":70,"hygiene":60,"mood":90,"extra":1}`,
	} {
		if _, err := pool.Exec(
			ctx,
			`UPDATE pets SET needs = $2::jsonb WHERE id = $1`,
			integrationActivePet,
			invalid,
		); err == nil {
			t.Fatalf("needs constraint accepted %s", invalid)
		}
	}
}

func TestNewPostgresStoreRequiresPool(t *testing.T) {
	if _, err := NewPostgresStore(nil); err == nil {
		t.Fatal("NewPostgresStore accepted nil pool")
	}
}

func TestPetJSONDecodersFailClosed(t *testing.T) {
	for _, input := range []string{
		`{}`,
		`{"hunger":101,"energy":70,"hygiene":60,"mood":90}`,
		`{"hunger":80,"energy":70,"hygiene":60,"mood":90,"extra":1}`,
	} {
		if _, err := decodeNeeds([]byte(input)); err == nil {
			t.Fatalf("decodeNeeds accepted %s", input)
		}
	}
	for _, input := range []string{
		`{}`,
		`{"str":1,"agi":2,"end":3,"foc":4,"extra":5}`,
		`{"str":4294967296,"agi":2,"end":3,"foc":4}`,
	} {
		if _, err := decodeStats([]byte(input)); err == nil {
			t.Fatalf("decodeStats accepted %s", input)
		}
	}
}

func profilePostgresTestPool(
	t *testing.T,
	ctx context.Context,
	databaseURL string,
) *pgxpool.Pool {
	t.Helper()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open PostgreSQL admin pool: %v", err)
	}
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		t.Fatalf("ping PostgreSQL: %v", err)
	}
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		admin.Close()
		t.Fatalf("generate schema name: %v", err)
	}
	schema := "gochya_profile_" + hex.EncodeToString(randomBytes)
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		admin.Close()
		t.Fatalf("create test schema: %v", err)
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		admin.Close()
		t.Fatalf("parse PostgreSQL config: %v", err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	config.ConnConfig.RuntimeParams["timezone"] = "UTC"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		admin.Close()
		t.Fatalf("open schema pool: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		admin.Close()
		t.Fatalf("ping schema pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer cleanupCancel()
		if _, err := admin.Exec(
			cleanupCtx,
			"DROP SCHEMA "+identifier+" CASCADE",
		); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
		admin.Close()
	})
	for _, path := range []string{
		"../../migrations/000000_base.up.sql",
		"../../migrations/000006_loadouts.up.sql",
		"../../migrations/000007_profile_pets_read.up.sql",
	} {
		migration, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration %q: %v", path, err)
		}
		if _, err := pool.Exec(ctx, string(migration)); err != nil {
			t.Fatalf("apply migration %q: %v", path, err)
		}
	}
	return pool
}

func seedProfileData(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	now time.Time,
) {
	t.Helper()
	if _, err := pool.Exec(
		ctx,
		`INSERT INTO players (
		     id, username, display_name, created_at, last_seen, auth_method,
		     auth_subject, timezone, streak_days, streak_last_day
		 ) VALUES
		     ($1, 'profile-player', 'Profile Player', $3, $4, 'google',
		      'profile-subject', 'Asia/Tbilisi', 4, '2026-07-23'),
		     ($2, 'other-player', NULL, $3, NULL, 'google',
		      'other-subject', NULL, 0, NULL)`,
		testPlayerID,
		integrationOtherPlayer,
		now.Add(-24*time.Hour),
		now.Add(-time.Hour),
	); err != nil {
		t.Fatalf("seed players: %v", err)
	}
	if _, err := pool.Exec(
		ctx,
		`INSERT INTO pets (
		     id, owner_id, genome, name, stage, level, xp, needs, stats,
		     generation, is_active, created_at
		 ) VALUES
		     ($1, $4, '{"element":"Earth"}', 'Active', 'baby', 3, 120,
		      '{"hunger":80,"energy":70,"hygiene":60,"mood":90}',
		      '{"str":11,"agi":12,"end":13,"foc":14}',
		      0, TRUE, $7),
		     ($2, $4, '{"element":"Water"}', 'Second', 'teen', 12, 900,
		      '{"hunger":50,"energy":55,"hygiene":60,"mood":65}',
		      '{"str":21,"agi":22,"end":23,"foc":24}',
		      1, FALSE, $6),
		     ($3, $5, '{"element":"Fire"}', 'Foreign', 'adult', 30, 5000,
		      '{"hunger":90,"energy":90,"hygiene":90,"mood":90}',
		      '{"str":31,"agi":32,"end":33,"foc":34}',
		      2, TRUE, $6)`,
		integrationActivePet,
		integrationSecondPet,
		integrationForeignPet,
		testPlayerID,
		integrationOtherPlayer,
		now.Add(-48*time.Hour),
		now.Add(-24*time.Hour),
	); err != nil {
		t.Fatalf("seed pets: %v", err)
	}
	cardIDs := []string{
		"40000000-0000-4000-8000-000000000001",
		"40000000-0000-4000-8000-000000000002",
		"40000000-0000-4000-8000-000000000003",
		"40000000-0000-4000-8000-000000000004",
		"40000000-0000-4000-8000-000000000005",
	}
	if _, err := pool.Exec(
		ctx,
		`INSERT INTO player_loadouts (
		     player_id, pet_id, card_ids, signature_idx, revision, updated_at
		 ) VALUES ($1, $2, $3::uuid[], 2, 7, $4)`,
		testPlayerID,
		integrationActivePet,
		cardIDs,
		now.Add(-time.Hour),
	); err != nil {
		t.Fatalf("seed loadout: %v", err)
	}
}
