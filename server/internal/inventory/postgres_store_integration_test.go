package inventory

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gochya/gochya/server/internal/dojo"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresTechniqueInventoryCursorPagination(t *testing.T) {
	databaseURL := os.Getenv("GOCHYA_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("GOCHYA_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := inventoryPostgresTestPool(t, ctx, databaseURL)
	seedInventoryPlayers(t, ctx, pool)

	now := time.Now().UTC().Truncate(time.Microsecond)
	cardIDs := []string{
		"11111111-1111-4111-8111-111111111111",
		"22222222-2222-4222-8222-222222222222",
		"33333333-3333-4333-8333-333333333333",
	}
	seedTechniqueCard(t, ctx, pool, cardIDs[0], inventoryTestPlayer, now.Add(-time.Minute), 60)
	seedTechniqueCard(t, ctx, pool, cardIDs[1], inventoryTestPlayer, now, 70)
	seedTechniqueCard(t, ctx, pool, cardIDs[2], inventoryTestPlayer, now, 80)
	seedTechniqueCard(
		t,
		ctx,
		pool,
		"44444444-4444-4444-8444-444444444444",
		"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		now.Add(time.Minute),
		100,
	)

	store, err := NewPostgresStore(pool)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	service, err := NewService(store)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	first, err := service.ListTechniques(ctx, inventoryTestPlayer, "2", "")
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(first.Items) != 2 ||
		first.Items[0].ID != cardIDs[2] ||
		first.Items[1].ID != cardIDs[1] ||
		first.NextCursor == "" {
		t.Fatalf("first page = %#v", first)
	}
	second, err := service.ListTechniques(
		ctx,
		inventoryTestPlayer,
		"2",
		first.NextCursor,
	)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second.Items) != 1 ||
		second.Items[0].ID != cardIDs[0] ||
		second.NextCursor != "" {
		t.Fatalf("second page = %#v", second)
	}
	if second.Items[0].OwnerID != inventoryTestPlayer ||
		second.Items[0].Quality != 60 {
		t.Fatalf("authoritative card = %#v", second.Items[0])
	}

	var indexDefinition string
	if err := pool.QueryRow(
		ctx,
		`SELECT indexdef
		   FROM pg_indexes
		  WHERE schemaname = current_schema()
		    AND indexname = 'idx_technique_cards_owner_created'`,
	).Scan(&indexDefinition); err != nil {
		t.Fatalf("query pagination index: %v", err)
	}
	if !strings.Contains(indexDefinition, "id DESC") {
		t.Fatalf("pagination index = %q", indexDefinition)
	}

	downMigration, err := os.ReadFile("../../migrations/000005_technique_pagination.down.sql")
	if err != nil {
		t.Fatalf("read pagination down migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(downMigration)); err != nil {
		t.Fatalf("apply pagination down migration: %v", err)
	}
}

func TestPostgresEquipIsAtomicAndIdempotent(t *testing.T) {
	databaseURL := os.Getenv("GOCHYA_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("GOCHYA_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool := inventoryPostgresTestPool(t, ctx, databaseURL)
	seedInventoryPlayers(t, ctx, pool)
	if _, err := pool.Exec(
		ctx,
		`INSERT INTO pets (
		     id, owner_id, genome, stage, needs, stats, is_active, created_at
		 ) VALUES (
		     'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb',
		     $1,
		     '{"element":"Earth"}',
		     'baby',
		     '{}',
		     '{}',
		     TRUE,
		     NOW()
		 )`,
		inventoryTestPlayer,
	); err != nil {
		t.Fatalf("seed active pet: %v", err)
	}
	cardIDs := []string{
		"10000000-0000-4000-8000-000000000001",
		"10000000-0000-4000-8000-000000000002",
		"10000000-0000-4000-8000-000000000003",
		"10000000-0000-4000-8000-000000000004",
		"10000000-0000-4000-8000-000000000005",
		"10000000-0000-4000-8000-000000000006",
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	for index, cardID := range cardIDs {
		seedTechniqueCard(
			t,
			ctx,
			pool,
			cardID,
			inventoryTestPlayer,
			now.Add(time.Duration(index)*time.Second),
			uint8(60+index),
		)
	}
	foreignCardID := "20000000-0000-4000-8000-000000000001"
	seedTechniqueCard(
		t,
		ctx,
		pool,
		foreignCardID,
		"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		now,
		90,
	)

	store, err := NewPostgresStore(pool)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	service, err := NewService(store)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	request := EquipTechniquesRequest{
		CardIDs:      append([]string(nil), cardIDs[:5]...),
		SignatureIdx: 2,
	}
	const concurrency = 8
	const idempotencyKey = "30000000-0000-4000-8000-000000000001"
	responses := make([]LoadoutResponse, concurrency)
	failures := make([]error, concurrency)
	var group sync.WaitGroup
	for index := range responses {
		group.Add(1)
		go func() {
			defer group.Done()
			responses[index], failures[index] = service.EquipTechniques(
				ctx,
				inventoryTestPlayer,
				idempotencyKey,
				request,
			)
		}()
	}
	group.Wait()
	for index, failure := range failures {
		if failure != nil {
			t.Fatalf("equip %d: %v", index, failure)
		}
		if !reflect.DeepEqual(responses[0], responses[index]) {
			t.Fatalf("equip %d returned another response", index)
		}
	}
	if responses[0].Revision != 1 ||
		responses[0].PetID != "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb" ||
		!reflect.DeepEqual(responses[0].CardIDs, request.CardIDs) {
		t.Fatalf("equip response = %#v", responses[0])
	}
	current, err := service.CurrentLoadout(ctx, inventoryTestPlayer)
	if err != nil {
		t.Fatalf("CurrentLoadout: %v", err)
	}
	if !reflect.DeepEqual(current, responses[0]) {
		t.Fatalf("current = %#v, equipped = %#v", current, responses[0])
	}
	assertLoadoutProjection(t, ctx, pool, inventoryTestPlayer, cardIDs[2], 5)
	var idempotencyCount int
	if err := pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM loadout_idempotency WHERE player_id = $1`,
		inventoryTestPlayer,
	).Scan(&idempotencyCount); err != nil {
		t.Fatalf("count loadout idempotency: %v", err)
	}
	if idempotencyCount != 1 {
		t.Fatalf("loadout idempotency count = %d", idempotencyCount)
	}

	changed := request
	changed.CardIDs = append([]string(nil), cardIDs[1:6]...)
	_, err = service.EquipTechniques(
		ctx,
		inventoryTestPlayer,
		idempotencyKey,
		changed,
	)
	assertInventoryErrorCode(t, err, "idempotency_conflict")

	foreign := request
	foreign.CardIDs = append([]string(nil), request.CardIDs...)
	foreign.CardIDs[4] = foreignCardID
	_, err = service.EquipTechniques(
		ctx,
		inventoryTestPlayer,
		"30000000-0000-4000-8000-000000000002",
		foreign,
	)
	assertInventoryErrorCode(t, err, "loadout_cards_invalid")
	currentAfterConflict, err := service.CurrentLoadout(ctx, inventoryTestPlayer)
	if err != nil {
		t.Fatalf("CurrentLoadout after conflict: %v", err)
	}
	if !reflect.DeepEqual(currentAfterConflict, current) {
		t.Fatal("failed equip changed current loadout")
	}

	changed.SignatureIdx = 4
	second, err := service.EquipTechniques(
		ctx,
		inventoryTestPlayer,
		"30000000-0000-4000-8000-000000000003",
		changed,
	)
	if err != nil {
		t.Fatalf("second equip: %v", err)
	}
	if second.Revision != 2 || !reflect.DeepEqual(second.CardIDs, changed.CardIDs) {
		t.Fatalf("second equip response = %#v", second)
	}
	assertLoadoutProjection(t, ctx, pool, inventoryTestPlayer, cardIDs[5], 5)

	downMigration, err := os.ReadFile("../../migrations/000006_loadouts.down.sql")
	if err != nil {
		t.Fatalf("read loadout down migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(downMigration)); err != nil {
		t.Fatalf("apply loadout down migration: %v", err)
	}
	var loadoutTable *string
	if err := pool.QueryRow(
		ctx,
		`SELECT to_regclass('player_loadouts')::text`,
	).Scan(&loadoutTable); err != nil {
		t.Fatalf("check loadout down migration: %v", err)
	}
	if loadoutTable != nil {
		t.Fatalf("down migration left table %q", *loadoutTable)
	}
}

func inventoryPostgresTestPool(
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
	schema := "gochya_inventory_" + hex.EncodeToString(randomBytes)
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
	baseMigration, err := os.ReadFile("../../migrations/000000_base.up.sql")
	if err != nil {
		t.Fatalf("read base migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(baseMigration)); err != nil {
		t.Fatalf("apply base migration: %v", err)
	}
	paginationMigration, err := os.ReadFile(
		"../../migrations/000005_technique_pagination.up.sql",
	)
	if err != nil {
		t.Fatalf("read pagination migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(paginationMigration)); err != nil {
		t.Fatalf("apply pagination migration: %v", err)
	}
	loadoutMigration, err := os.ReadFile("../../migrations/000006_loadouts.up.sql")
	if err != nil {
		t.Fatalf("read loadout migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(loadoutMigration)); err != nil {
		t.Fatalf("apply loadout migration: %v", err)
	}
	return pool
}

func seedInventoryPlayers(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(
		ctx,
		`INSERT INTO players (
		     id, username, created_at, auth_method, auth_subject
		 ) VALUES
		     ($1, 'inventory-player', NOW(), 'google', 'inventory-subject'),
		     ($2, 'other-player', NOW(), 'google', 'other-subject')`,
		inventoryTestPlayer,
		"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
	); err != nil {
		t.Fatalf("seed players: %v", err)
	}
}

func seedTechniqueCard(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	id string,
	ownerID string,
	createdAt time.Time,
	quality uint8,
) {
	t.Helper()
	cardJSON, err := json.Marshal(dojo.TechniqueCard{
		ID:          "ffffffff-ffff-4fff-8fff-ffffffffffff",
		OwnerID:     "ffffffff-ffff-4fff-8fff-ffffffffffff",
		Type:        1,
		Element:     2,
		Rarity:      3,
		BaseDamage:  1.2,
		Speed:       2.3,
		StaminaCost: 4,
		CritChance:  0.2,
		Quality:     quality,
		CreatedAt:   createdAt.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("marshal card: %v", err)
	}
	if _, err := pool.Exec(
		ctx,
		`INSERT INTO technique_cards (
		     id, owner_id, card_data, is_equipped, is_signature, created_at
		 ) VALUES ($1, $2, $3, FALSE, FALSE, $4)`,
		id,
		ownerID,
		cardJSON,
		createdAt,
	); err != nil {
		t.Fatalf("seed technique card: %v", err)
	}
}

func assertLoadoutProjection(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	playerID string,
	signatureID string,
	wantEquipped int,
) {
	t.Helper()
	var equipped int
	var storedSignatureID string
	if err := pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FILTER (WHERE is_equipped),
		        COALESCE(MAX(id::text) FILTER (WHERE is_signature), '')
		   FROM technique_cards
		  WHERE owner_id = $1`,
		playerID,
	).Scan(&equipped, &storedSignatureID); err != nil {
		t.Fatalf("query loadout projection: %v", err)
	}
	if equipped != wantEquipped || storedSignatureID != signatureID {
		t.Fatalf(
			"loadout projection equipped/signature = %d/%q, want %d/%q",
			equipped,
			storedSignatureID,
			wantEquipped,
			signatureID,
		)
	}
}

func assertInventoryErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != code {
		t.Fatalf("error = %v, want code %q", err, code)
	}
}
