package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gochya/gochya/server/internal/ledgeraudit"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	auditTimeout       = 30 * time.Second
	exitHealthy        = 0
	exitRuntimeFailure = 1
	exitInconsistent   = 2
)

type environmentLookup func(string) (string, bool)

type auditRunner func(
	context.Context,
	string,
) (ledgeraudit.Report, error)

func main() {
	signalCtx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()
	ctx, cancel := context.WithTimeout(signalCtx, auditTimeout)
	defer cancel()
	os.Exit(run(
		ctx,
		os.LookupEnv,
		os.Stdout,
		os.Stderr,
		runPostgresAudit,
	))
}

func run(
	ctx context.Context,
	lookup environmentLookup,
	stdout io.Writer,
	stderr io.Writer,
	audit auditRunner,
) int {
	databaseURL, err := requiredDatabaseURL(lookup)
	if err != nil {
		fmt.Fprintf(stderr, "ledger audit: %v\n", err)
		return exitRuntimeFailure
	}
	report, err := audit(ctx, databaseURL)
	if err != nil {
		fmt.Fprintf(stderr, "ledger audit: %v\n", err)
		return exitRuntimeFailure
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintf(stderr, "ledger audit: encode report: %v\n", err)
		return exitRuntimeFailure
	}
	if !report.Healthy {
		return exitInconsistent
	}
	return exitHealthy
}

func requiredDatabaseURL(lookup environmentLookup) (string, error) {
	value, ok := lookup("GOCHYA_DATABASE_URL")
	value = strings.TrimSpace(value)
	if !ok || value == "" {
		return "", errors.New("GOCHYA_DATABASE_URL is required")
	}
	return value, nil
}

func runPostgresAudit(
	ctx context.Context,
	databaseURL string,
) (ledgeraudit.Report, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return ledgeraudit.Report{}, fmt.Errorf(
			"parse PostgreSQL configuration: %w",
			err,
		)
	}
	config.ConnConfig.RuntimeParams["application_name"] = "gochya-ledger-audit"
	config.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return ledgeraudit.Report{}, fmt.Errorf(
			"create PostgreSQL pool: %w",
			err,
		)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return ledgeraudit.Report{}, fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	auditor, err := ledgeraudit.NewPostgresAuditor(
		ledgeraudit.PostgresAuditorConfig{Pool: pool},
	)
	if err != nil {
		return ledgeraudit.Report{}, fmt.Errorf("create auditor: %w", err)
	}
	return auditor.Audit(ctx)
}
