package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestProductionMigrationsBootstrapEmptySchema(t *testing.T) {
	databaseURL := os.Getenv("GOCHYA_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("GOCHYA_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := schemaValidationTestPool(t, ctx, databaseURL)

	upMigrations := applyAllUpMigrations(t, ctx, pool)

	if err := validateDatabaseSchema(ctx, pool); err != nil {
		t.Fatalf("validate migrated schema: %v", err)
	}

	const playerID = "11111111-1111-4111-8111-111111111111"
	if _, err := pool.Exec(
		ctx,
		`INSERT INTO players (
		     id, username, created_at, auth_method, auth_subject
		 ) VALUES ($1, 'migration-player', NOW(), 'google', 'migration-subject')`,
		playerID,
	); err != nil {
		t.Fatalf("insert player through production schema: %v", err)
	}
	if _, err := pool.Exec(
		ctx,
		`INSERT INTO pets (
		     id, owner_id, genome, stage, needs, stats, is_active
		 ) VALUES (
		     '22222222-2222-4222-8222-222222222222',
		     $1,
		     '{"element":"earth"}',
		     'baby',
		     '{"hunger":80,"energy":70,"hygiene":60,"mood":90}',
		     '{"str":1,"agi":2,"end":3,"foc":4}',
		     TRUE
		 )`,
		playerID,
	); err != nil {
		t.Fatalf("insert pet through production schema: %v", err)
	}

	downMigrations, err := filepath.Glob("../../migrations/*.down.sql")
	if err != nil {
		t.Fatalf("find down migrations: %v", err)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(downMigrations)))
	if len(downMigrations) != len(upMigrations) {
		t.Fatalf(
			"migration direction count differs: up=%d down=%d",
			len(upMigrations),
			len(downMigrations),
		)
	}
	for _, path := range downMigrations {
		migration, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration %q: %v", path, err)
		}
		if _, err := pool.Exec(ctx, string(migration)); err != nil {
			t.Fatalf("apply migration %q: %v", path, err)
		}
	}
	var playersTable *string
	if err := pool.QueryRow(
		ctx,
		`SELECT to_regclass('players')::text`,
	).Scan(&playersTable); err != nil {
		t.Fatalf("check base down migration: %v", err)
	}
	if playersTable != nil {
		t.Fatalf("down migrations left table %q", *playersTable)
	}
}

func applyAllUpMigrations(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
) []string {
	t.Helper()
	upMigrations, err := filepath.Glob("../../migrations/*.up.sql")
	if err != nil {
		t.Fatalf("find up migrations: %v", err)
	}
	sort.Strings(upMigrations)
	if len(upMigrations) == 0 {
		t.Fatal("no up migrations found")
	}
	for _, path := range upMigrations {
		migration, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration %q: %v", path, err)
		}
		if _, err := pool.Exec(ctx, string(migration)); err != nil {
			t.Fatalf("apply migration %q: %v", path, err)
		}
	}
	return upMigrations
}

func TestValidateDatabaseSchema(t *testing.T) {
	databaseURL := os.Getenv("GOCHYA_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("GOCHYA_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := schemaValidationTestPool(t, ctx, databaseURL)

	err := validateDatabaseSchema(ctx, pool)
	if err == nil || !strings.Contains(err.Error(), "refresh_tokens") {
		t.Fatalf("missing-schema error = %v", err)
	}
	for _, table := range requiredDatabaseTables {
		if _, err := pool.Exec(
			ctx,
			"CREATE TABLE "+pgx.Identifier{table}.Sanitize()+" ()",
		); err != nil {
			t.Fatalf("create required table %q: %v", table, err)
		}
	}
	if err := validateDatabaseSchema(ctx, pool); err != nil {
		t.Fatalf("validate complete schema: %v", err)
	}
}

func schemaValidationTestPool(
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
		t.Fatalf("generate schema name: %v", err)
	}
	schema := "gochya_startup_" + hex.EncodeToString(randomBytes)
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
	return pool
}
