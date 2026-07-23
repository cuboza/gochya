package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

var requiredDatabaseTables = []string{
	"players",
	"pets",
	"technique_cards",
	"registered_devices",
	"dojo_nonces",
	"dojo_replay_hashes",
	"idempotency_results",
	"dojo_submission_audit",
	"refresh_tokens",
}

func validateDatabaseSchema(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := pool.Query(
		ctx,
		`SELECT required_table
		   FROM unnest($1::text[]) AS required(required_table)
		  WHERE to_regclass(required_table) IS NULL`,
		requiredDatabaseTables,
	)
	if err != nil {
		return fmt.Errorf("query required database tables: %w", err)
	}
	defer rows.Close()
	var missing []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return fmt.Errorf("scan missing database table: %w", err)
		}
		missing = append(missing, table)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate required database tables: %w", err)
	}
	if len(missing) > 0 {
		return errors.New("database migrations are missing tables: " + strings.Join(missing, ", "))
	}
	return nil
}
