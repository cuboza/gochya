package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

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
