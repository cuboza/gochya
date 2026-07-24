package ledgeraudit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresAuditorConfig struct {
	Pool *pgxpool.Pool
	Now  func() time.Time
}

type PostgresAuditor struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func NewPostgresAuditor(config PostgresAuditorConfig) (*PostgresAuditor, error) {
	if config.Pool == nil {
		return nil, errors.New("PostgreSQL pool is required")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &PostgresAuditor{pool: config.Pool, now: config.Now}, nil
}

func (a *PostgresAuditor) Audit(ctx context.Context) (Report, error) {
	tx, err := a.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return Report{}, fmt.Errorf("begin ledger audit: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	report := Report{
		CheckedAt:         a.now().UTC().Truncate(time.Microsecond),
		UnbalancedEntries: make([]UnbalancedEntry, 0),
		Mismatches:        make([]Mismatch, 0),
	}
	if err := tx.QueryRow(ctx, `SELECT
		(SELECT COUNT(*) * 2 FROM player_wallet),
		(SELECT COUNT(*) FROM player_items),
		(SELECT COUNT(*) FROM transactions) +
			(SELECT COUNT(*) FROM item_transactions)`,
	).Scan(
		&report.CurrencyProjectionsChecked,
		&report.ItemProjectionsChecked,
		&report.LedgerEntriesChecked,
	); err != nil {
		return Report{}, fmt.Errorf("count audited ledger rows: %w", err)
	}

	rows, err := tx.Query(ctx, unbalancedQuery)
	if err != nil {
		return Report{}, fmt.Errorf("query unbalanced ledger entries: %w", err)
	}
	for rows.Next() {
		var entry UnbalancedEntry
		if err := rows.Scan(
			&entry.Kind,
			&entry.EntryID,
			&entry.PlayerID,
			&entry.Asset,
			&entry.Amount,
			&entry.CounterpartyAmount,
		); err != nil {
			rows.Close()
			return Report{}, fmt.Errorf("scan unbalanced ledger entry: %w", err)
		}
		report.UnbalancedEntries = append(report.UnbalancedEntries, entry)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Report{}, fmt.Errorf("iterate unbalanced ledger entries: %w", err)
	}
	rows.Close()

	rows, err = tx.Query(ctx, mismatchQuery)
	if err != nil {
		return Report{}, fmt.Errorf("query ledger mismatches: %w", err)
	}
	for rows.Next() {
		var mismatch Mismatch
		if err := rows.Scan(
			&mismatch.Kind,
			&mismatch.PlayerID,
			&mismatch.Asset,
			&mismatch.Scope,
			&mismatch.ProjectionBalance,
			&mismatch.LedgerBalance,
		); err != nil {
			rows.Close()
			return Report{}, fmt.Errorf("scan ledger mismatch: %w", err)
		}
		report.Mismatches = append(report.Mismatches, mismatch)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Report{}, fmt.Errorf("iterate ledger mismatches: %w", err)
	}
	rows.Close()
	report.finalize()
	if err := tx.Commit(ctx); err != nil {
		return Report{}, fmt.Errorf("commit ledger audit snapshot: %w", err)
	}
	return report, nil
}

const unbalancedQuery = `SELECT
	kind,entry_id,player_id,asset,amount,counterparty_amount
FROM (
	SELECT
		'currency'::TEXT AS kind,
		id::BIGINT AS entry_id,
		player_id::TEXT AS player_id,
		currency::TEXT AS asset,
		amount::BIGINT AS amount,
		counterparty_amount::BIGINT AS counterparty_amount
	FROM transactions
	WHERE amount::NUMERIC + counterparty_amount::NUMERIC <> 0
	UNION ALL
	SELECT
		'item'::TEXT AS kind,
		id::BIGINT AS entry_id,
		player_id::TEXT AS player_id,
		item_id::TEXT AS asset,
		amount::BIGINT AS amount,
		counterparty_amount::BIGINT AS counterparty_amount
	FROM item_transactions
	WHERE amount::BIGINT + counterparty_amount::BIGINT <> 0
) entries
ORDER BY kind ASC,entry_id ASC`

const mismatchQuery = `WITH
koins_ledger AS (
	SELECT player_id,SUM(amount)::BIGINT AS balance
	FROM transactions
	WHERE currency='koins'
	GROUP BY player_id
),
koins_mismatches AS (
	SELECT
		'currency'::TEXT AS kind,
		p.id::TEXT AS player_id,
		'koins'::TEXT AS asset,
		''::TEXT AS scope,
		COALESCE(w.koins,0)::BIGINT AS projection_balance,
		COALESCE(l.balance,0)::BIGINT AS ledger_balance
	FROM players p
	LEFT JOIN player_wallet w ON w.player_id=p.id
	LEFT JOIN koins_ledger l ON l.player_id=p.id
	WHERE COALESCE(w.koins,0) <> COALESCE(l.balance,0)
),
vitality_ledger AS (
	SELECT player_id,ref_id,SUM(amount)::BIGINT AS balance
	FROM transactions
	WHERE currency='vitality'
	GROUP BY player_id,ref_id
),
vitality_mismatches AS (
	SELECT
		'currency'::TEXT AS kind,
		w.player_id::TEXT AS player_id,
		'vitality'::TEXT AS asset,
		w.vitality_date::TEXT AS scope,
		w.vitality_daily::BIGINT AS projection_balance,
		COALESCE(l.balance,0)::BIGINT AS ledger_balance
	FROM player_wallet w
	LEFT JOIN vitality_ledger l
		ON l.player_id=w.player_id
		AND l.ref_id=w.vitality_date::TEXT
	WHERE w.vitality_daily::BIGINT <> COALESCE(l.balance,0)
),
orphan_vitality_mismatches AS (
	SELECT
		'currency'::TEXT AS kind,
		l.player_id::TEXT AS player_id,
		'vitality'::TEXT AS asset,
		l.ref_id::TEXT AS scope,
		0::BIGINT AS projection_balance,
		l.balance::BIGINT AS ledger_balance
	FROM vitality_ledger l
	LEFT JOIN player_wallet w ON w.player_id=l.player_id
	WHERE w.player_id IS NULL
),
item_ledger AS (
	SELECT player_id,item_id,SUM(amount)::BIGINT AS balance
	FROM item_transactions
	GROUP BY player_id,item_id
),
item_mismatches AS (
	SELECT
		'item'::TEXT AS kind,
		COALESCE(i.player_id,l.player_id)::TEXT AS player_id,
		COALESCE(i.item_id,l.item_id)::TEXT AS asset,
		''::TEXT AS scope,
		COALESCE(i.quantity,0)::BIGINT AS projection_balance,
		COALESCE(l.balance,0)::BIGINT AS ledger_balance
	FROM player_items i
	FULL OUTER JOIN item_ledger l
		ON l.player_id=i.player_id AND l.item_id=i.item_id
	WHERE COALESCE(i.quantity,0)::BIGINT <> COALESCE(l.balance,0)
)
SELECT kind,player_id,asset,scope,projection_balance,ledger_balance
FROM (
	SELECT * FROM koins_mismatches
	UNION ALL
	SELECT * FROM vitality_mismatches
	UNION ALL
	SELECT * FROM orphan_vitality_mismatches
	UNION ALL
	SELECT * FROM item_mismatches
) mismatches
ORDER BY kind ASC,player_id ASC,asset ASC,scope ASC`
