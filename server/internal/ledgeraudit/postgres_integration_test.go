package ledgeraudit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	auditPlayerID = "71000000-0000-4000-8000-000000000001"
	emptyPlayerID = "71000000-0000-4000-8000-000000000002"
)

func TestPostgresAuditorDetectsProjectionDriftAndUnbalancedEntries(
	t *testing.T,
) {
	databaseURL := os.Getenv("GOCHYA_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("GOCHYA_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool := ledgerAuditPostgresPool(t, ctx, databaseURL)
	now := time.Date(2026, time.July, 24, 18, 0, 0, 123_456_000, time.UTC)
	seedHealthyLedger(t, ctx, pool, now)
	auditor, err := NewPostgresAuditor(PostgresAuditorConfig{
		Pool: pool,
		Now:  func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewPostgresAuditor: %v", err)
	}

	report, err := auditor.Audit(ctx)
	if err != nil {
		t.Fatalf("healthy Audit: %v", err)
	}
	if !report.Healthy ||
		report.CheckedAt != now ||
		report.CurrencyProjectionsChecked != 2 ||
		report.ItemProjectionsChecked != 1 ||
		report.LedgerEntriesChecked != 6 ||
		len(report.UnbalancedEntries) != 0 ||
		len(report.Mismatches) != 0 {
		t.Fatalf("healthy report = %#v", report)
	}

	if _, err := pool.Exec(ctx, `UPDATE player_wallet
		SET koins=101,vitality_daily=41
		WHERE player_id=$1`,
		auditPlayerID,
	); err != nil {
		t.Fatalf("corrupt wallet projection: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE player_items SET quantity=4
		WHERE player_id=$1 AND item_id='apple'`,
		auditPlayerID,
	); err != nil {
		t.Fatalf("corrupt item projection: %v", err)
	}
	report, err = auditor.Audit(ctx)
	if err != nil {
		t.Fatalf("projection drift Audit: %v", err)
	}
	wantMismatches := []Mismatch{
		{
			Kind:              KindCurrency,
			PlayerID:          auditPlayerID,
			Asset:             "koins",
			ProjectionBalance: 101,
			LedgerBalance:     100,
		},
		{
			Kind:              KindCurrency,
			PlayerID:          auditPlayerID,
			Asset:             "vitality",
			Scope:             "2026-07-24",
			ProjectionBalance: 41,
			LedgerBalance:     40,
		},
		{
			Kind:              KindItem,
			PlayerID:          auditPlayerID,
			Asset:             "apple",
			ProjectionBalance: 4,
			LedgerBalance:     2,
		},
	}
	if report.Healthy ||
		len(report.UnbalancedEntries) != 0 ||
		len(report.Mismatches) != len(wantMismatches) {
		t.Fatalf("projection drift report = %#v", report)
	}
	for index := range wantMismatches {
		if report.Mismatches[index] != wantMismatches[index] {
			t.Fatalf(
				"mismatch %d = %#v, want %#v",
				index,
				report.Mismatches[index],
				wantMismatches[index],
			)
		}
	}

	if _, err := pool.Exec(ctx, `UPDATE player_wallet
		SET koins=100,vitality_daily=40
		WHERE player_id=$1`,
		auditPlayerID,
	); err != nil {
		t.Fatalf("repair wallet projection: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE player_items SET quantity=2
		WHERE player_id=$1 AND item_id='apple'`,
		auditPlayerID,
	); err != nil {
		t.Fatalf("repair item projection: %v", err)
	}
	dropBalanceConstraint(t, ctx, pool, "transactions")
	dropBalanceConstraint(t, ctx, pool, "item_transactions")
	if _, err := pool.Exec(ctx, `UPDATE transactions
		SET counterparty_amount=-149
		WHERE player_id=$1 AND idempotency_key='audit:koins:grant'`,
		auditPlayerID,
	); err != nil {
		t.Fatalf("corrupt currency double-entry row: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE item_transactions
		SET counterparty_amount=-2
		WHERE player_id=$1 AND idempotency_key='audit:item:grant'`,
		auditPlayerID,
	); err != nil {
		t.Fatalf("corrupt item double-entry row: %v", err)
	}
	report, err = auditor.Audit(ctx)
	if err != nil {
		t.Fatalf("unbalanced Audit: %v", err)
	}
	if report.Healthy ||
		len(report.Mismatches) != 0 ||
		len(report.UnbalancedEntries) != 2 {
		t.Fatalf("unbalanced report = %#v", report)
	}
	currencyEntry := report.UnbalancedEntries[0]
	if currencyEntry.Kind != KindCurrency ||
		currencyEntry.PlayerID != auditPlayerID ||
		currencyEntry.Asset != "koins" ||
		currencyEntry.Amount != 150 ||
		currencyEntry.CounterpartyAmount != -149 {
		t.Fatalf("currency unbalanced entry = %#v", currencyEntry)
	}
	itemEntry := report.UnbalancedEntries[1]
	if itemEntry.Kind != KindItem ||
		itemEntry.PlayerID != auditPlayerID ||
		itemEntry.Asset != "apple" ||
		itemEntry.Amount != 3 ||
		itemEntry.CounterpartyAmount != -2 {
		t.Fatalf("item unbalanced entry = %#v", itemEntry)
	}
}

func seedHealthyLedger(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	now time.Time,
) {
	t.Helper()
	if _, err := pool.Exec(ctx, `INSERT INTO players(
		id,username,auth_method,auth_subject)
		VALUES
		($1,'ledger-player','google','ledger-player'),
		($2,'empty-ledger-player','google','empty-ledger-player')`,
		auditPlayerID,
		emptyPlayerID,
	); err != nil {
		t.Fatalf("insert audit players: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO player_wallet(
		player_id,koins,vitality_daily,vitality_date,updated_at)
		VALUES($1,100,40,'2026-07-24',$2)`,
		auditPlayerID,
		now,
	); err != nil {
		t.Fatalf("insert healthy wallet: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO transactions(
			player_id,currency,amount,counterparty,counterparty_amount,
			reason,ref_id,idempotency_key,created_at)
		VALUES
		($1,'koins',150,'system:test',-150,
		 'test_grant','koins-grant','audit:koins:grant',$2),
		($1,'koins',-50,'system:test',50,
		 'test_spend','koins-spend','audit:koins:spend',$2),
		($1,'vitality',40,'system:test',-40,
		 'test_activity','2026-07-24','audit:vitality:current',$2),
		($1,'vitality',20,'system:test',-20,
		 'test_activity','2026-07-23','audit:vitality:historical',$2)`,
		auditPlayerID,
		now,
	); err != nil {
		t.Fatalf("insert healthy currency ledger: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO player_items(
			player_id,item_id,quantity,updated_at)
		VALUES($1,'apple',2,$2)`,
		auditPlayerID,
		now,
	); err != nil {
		t.Fatalf("insert healthy item projection: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO item_transactions(
			player_id,item_id,amount,counterparty,counterparty_amount,
			reason,ref_id,idempotency_key,created_at)
		VALUES
		($1,'apple',3,'system:test',-3,
		 'test_grant','apple-grant','audit:item:grant',$2),
		($1,'apple',-1,'system:test',1,
		 'test_spend','apple-spend','audit:item:spend',$2)`,
		auditPlayerID,
		now,
	); err != nil {
		t.Fatalf("insert healthy item ledger: %v", err)
	}
}

func dropBalanceConstraint(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	table string,
) {
	t.Helper()
	var constraint string
	if err := pool.QueryRow(ctx, `SELECT conname
		FROM pg_constraint
		WHERE conrelid=$1::regclass
		  AND contype='c'
		  AND POSITION('counterparty_amount' IN pg_get_constraintdef(oid)) > 0`,
		table,
	).Scan(&constraint); err != nil {
		t.Fatalf("find %s balance constraint: %v", table, err)
	}
	statement := "ALTER TABLE " +
		pgx.Identifier{table}.Sanitize() +
		" DROP CONSTRAINT " +
		pgx.Identifier{constraint}.Sanitize()
	if _, err := pool.Exec(ctx, statement); err != nil {
		t.Fatalf("drop %s balance constraint: %v", table, err)
	}
}

func ledgerAuditPostgresPool(
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
		t.Fatalf("generate ledger audit schema: %v", err)
	}
	schema := "gochya_ledger_audit_" + hex.EncodeToString(randomBytes)
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		admin.Close()
		t.Fatalf("create ledger audit schema: %v", err)
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
		t.Fatalf("open ledger audit schema: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer cleanupCancel()
		_, _ = admin.Exec(
			cleanupCtx,
			"DROP SCHEMA "+identifier+" CASCADE",
		)
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
