package shop

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresShopPurchasesAreAtomicAndIdempotent(t *testing.T) {
	databaseURL := os.Getenv("GOCHYA_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("GOCHYA_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool := shopPostgresPool(t, ctx, databaseURL)
	now := time.Date(2026, time.July, 24, 13, 0, 0, 123_456_000, time.UTC)
	seedShopData(t, ctx, pool, now)
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

	const concurrency = 8
	responses := make([]PurchaseResponse, concurrency)
	failures := make([]error, concurrency)
	var group sync.WaitGroup
	for index := range responses {
		group.Add(1)
		go func() {
			defer group.Done()
			responses[index], failures[index] = service.Purchase(
				ctx,
				testPlayerID,
				testKey,
				PurchaseRequest{ItemID: ItemApple, Quantity: 2},
			)
		}()
	}
	group.Wait()
	for index := range responses {
		if failures[index] != nil ||
			responses[index].ItemID != ItemApple ||
			responses[index].PurchasedQuantity != 2 ||
			responses[index].ItemQuantity != 2 ||
			responses[index].KoinsSpent != 40 ||
			responses[index].KoinsRemaining != 460 ||
			!responses[index].PurchasedAt.Equal(now) {
			t.Fatalf("purchase %d = %#v, %v", index, responses[index], failures[index])
		}
	}
	assertShopPersistence(t, ctx, pool, 460, 2, 0, 1, 2, 1)

	_, err = service.Purchase(
		ctx,
		testPlayerID,
		testKey,
		PurchaseRequest{ItemID: ItemSteak, Quantity: 2},
	)
	assertShopError(t, err, "idempotency_conflict")
	assertShopPersistence(t, ctx, pool, 460, 2, 0, 1, 2, 1)

	keys := []string{
		"33333333-3333-4333-8333-333333333333",
		"44444444-4444-4444-8444-444444444444",
	}
	competingResponses := make([]PurchaseResponse, len(keys))
	competingErrors := make([]error, len(keys))
	for index := range keys {
		group.Add(1)
		go func() {
			defer group.Done()
			competingResponses[index], competingErrors[index] = service.Purchase(
				ctx,
				testPlayerID,
				keys[index],
				PurchaseRequest{ItemID: ItemLoveCrystal, Quantity: 2},
			)
		}()
	}
	group.Wait()
	applied := 0
	rejected := 0
	var appliedKey string
	for index := range keys {
		switch {
		case competingErrors[index] == nil:
			applied++
			appliedKey = keys[index]
			if competingResponses[index].KoinsRemaining != 60 ||
				competingResponses[index].ItemQuantity != 2 {
				t.Fatalf("competing purchase %d = %#v", index, competingResponses[index])
			}
		default:
			rejected++
			assertShopError(t, competingErrors[index], "insufficient_koins")
		}
	}
	if applied != 1 || rejected != 1 {
		t.Fatalf("competing applied/rejected = %d/%d", applied, rejected)
	}
	assertShopPersistence(t, ctx, pool, 60, 2, 2, 2, 3, 2)

	repeated, err := service.Purchase(
		ctx,
		testPlayerID,
		testKey,
		PurchaseRequest{ItemID: ItemApple, Quantity: 2},
	)
	if err != nil || repeated.KoinsRemaining != 460 || repeated.ItemQuantity != 2 {
		t.Fatalf("late retry = %#v, %v", repeated, err)
	}
	repricedRetry, err := store.Purchase(ctx, PurchaseCommit{
		PlayerID:       testPlayerID,
		IdempotencyKey: testKey,
		RequestHash: sha256.Sum256(
			[]byte(`{"itemId":"apple","quantity":2}`),
		),
		Item: CatalogItem{
			ID:          ItemApple,
			Category:    CategoryCare,
			Currency:    CurrencyKoins,
			UnitPrice:   999,
			IsStackable: true,
		},
		Quantity: 2,
		Now:      now.Add(time.Hour),
	})
	if err != nil ||
		repricedRetry.UnitPriceKoins != 20 ||
		repricedRetry.KoinsSpent != 40 ||
		repricedRetry.KoinsRemaining != 460 {
		t.Fatalf("repriced retry = %#v, %v", repricedRetry, err)
	}
	repeatedCrystal, err := service.Purchase(
		ctx,
		testPlayerID,
		appliedKey,
		PurchaseRequest{ItemID: ItemLoveCrystal, Quantity: 2},
	)
	if err != nil || repeatedCrystal.KoinsRemaining != 60 ||
		repeatedCrystal.ItemQuantity != 2 {
		t.Fatalf("crystal retry = %#v, %v", repeatedCrystal, err)
	}
	assertShopPersistence(t, ctx, pool, 60, 2, 2, 2, 3, 2)

	inventory, err := service.Inventory(ctx, testPlayerID)
	if err != nil ||
		inventory.Koins != 60 ||
		len(inventory.Items) != 2 ||
		inventory.Items[0] != (OwnedItem{ItemID: ItemApple, Quantity: 2}) ||
		inventory.Items[1] != (OwnedItem{ItemID: ItemLoveCrystal, Quantity: 2}) {
		t.Fatalf("inventory = %#v, %v", inventory, err)
	}

	_, err = service.Purchase(
		ctx,
		"55555555-5555-4555-8555-555555555555",
		"66666666-6666-4666-8666-666666666666",
		PurchaseRequest{ItemID: ItemApple, Quantity: 1},
	)
	assertShopError(t, err, "insufficient_koins")
	var rejectedPurchases int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM shop_purchases
		WHERE player_id='55555555-5555-4555-8555-555555555555'`,
	).Scan(&rejectedPurchases); err != nil || rejectedPurchases != 0 {
		t.Fatalf("rejected purchases = %d, %v", rejectedPurchases, err)
	}
}

func assertShopPersistence(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	wantKoins int64,
	wantApples int,
	wantCrystals int,
	wantPurchases int,
	wantCoinLedger int,
	wantItemLedger int,
) {
	t.Helper()
	var koins, coinSum, coinBalance int64
	var apples, crystals, purchases, coinLedger, itemLedger int
	var itemSum, itemBalance int64
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT koins FROM player_wallet WHERE player_id=$1),
		COALESCE((SELECT quantity FROM player_items
		 WHERE player_id=$1 AND item_id='apple'),0),
		COALESCE((SELECT quantity FROM player_items
		 WHERE player_id=$1 AND item_id='love_crystal'),0),
		(SELECT COUNT(*) FROM shop_purchases WHERE player_id=$1),
		(SELECT COUNT(*) FROM transactions WHERE player_id=$1),
		(SELECT COUNT(*) FROM item_transactions WHERE player_id=$1),
		(SELECT COALESCE(SUM(amount),0) FROM transactions WHERE player_id=$1),
		(SELECT COALESCE(SUM(amount+counterparty_amount),0)
		 FROM transactions WHERE player_id=$1),
		(SELECT COALESCE(SUM(amount),0) FROM item_transactions WHERE player_id=$1),
		(SELECT COALESCE(SUM(amount+counterparty_amount),0)
		 FROM item_transactions WHERE player_id=$1)`,
		testPlayerID,
	).Scan(
		&koins,
		&apples,
		&crystals,
		&purchases,
		&coinLedger,
		&itemLedger,
		&coinSum,
		&coinBalance,
		&itemSum,
		&itemBalance,
	); err != nil {
		t.Fatalf("query shop persistence: %v", err)
	}
	if koins != wantKoins ||
		apples != wantApples ||
		crystals != wantCrystals ||
		purchases != wantPurchases ||
		coinLedger != wantCoinLedger ||
		itemLedger != wantItemLedger ||
		coinSum != wantKoins ||
		coinBalance != 0 ||
		itemSum != int64(wantApples+wantCrystals) ||
		itemBalance != 0 {
		t.Fatalf(
			"shop persistence = koins %d apples %d crystals %d purchases %d coin ledger/sum/balance %d/%d/%d item ledger/sum/balance %d/%d/%d",
			koins,
			apples,
			crystals,
			purchases,
			coinLedger,
			coinSum,
			coinBalance,
			itemLedger,
			itemSum,
			itemBalance,
		)
	}
}

func assertShopError(t *testing.T, err error, code string) {
	t.Helper()
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != code {
		t.Fatalf("error = %#v, want code %q", err, code)
	}
}

func seedShopData(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	now time.Time,
) {
	t.Helper()
	if _, err := pool.Exec(ctx, `INSERT INTO players(
		id,username,auth_method,auth_subject)
		VALUES
		($1,'shop-player','google','shop-player'),
		('55555555-5555-4555-8555-555555555555',
		 'empty-shop-player','google','empty-shop-player')`,
		testPlayerID,
	); err != nil {
		t.Fatalf("insert shop players: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO player_wallet(
		player_id,koins,vitality_daily,vitality_date,updated_at)
		VALUES($1,500,0,'2026-07-24',$2)`,
		testPlayerID,
		now,
	); err != nil {
		t.Fatalf("insert shop wallet: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO transactions(
		player_id,currency,amount,counterparty,counterparty_amount,
		reason,ref_id,idempotency_key,created_at)
		VALUES($1,'koins',500,'system:test',-500,
		'test_grant','shop-seed','test:shop-seed',$2)`,
		testPlayerID,
		now,
	); err != nil {
		t.Fatalf("insert shop seed ledger: %v", err)
	}
}

func shopPostgresPool(
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
		t.Fatalf("generate shop schema: %v", err)
	}
	schema := "gochya_shop_" + hex.EncodeToString(randomBytes)
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		admin.Close()
		t.Fatalf("create shop schema: %v", err)
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
		t.Fatalf("open shop schema: %v", err)
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
