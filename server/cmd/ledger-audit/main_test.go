package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gochya/gochya/server/internal/ledgeraudit"
)

func TestRunReturnsHealthyReport(t *testing.T) {
	var stdout, stderr bytes.Buffer
	now := time.Date(2026, time.July, 24, 18, 0, 0, 0, time.UTC)
	called := false
	exitCode := run(
		context.Background(),
		mapLookup(map[string]string{
			"GOCHYA_DATABASE_URL": " postgres://gochya@db/gochya ",
		}),
		&stdout,
		&stderr,
		func(_ context.Context, databaseURL string) (ledgeraudit.Report, error) {
			called = true
			if databaseURL != "postgres://gochya@db/gochya" {
				t.Fatalf("databaseURL = %q", databaseURL)
			}
			return ledgeraudit.Report{
				CheckedAt:         now,
				Healthy:           true,
				UnbalancedEntries: []ledgeraudit.UnbalancedEntry{},
				Mismatches:        []ledgeraudit.Mismatch{},
			}, nil
		},
	)
	if exitCode != exitHealthy {
		t.Fatalf("exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	if !called || stderr.Len() != 0 {
		t.Fatalf("called = %v, stderr = %q", called, stderr.String())
	}
	for _, fragment := range []string{
		`"checkedAt": "2026-07-24T18:00:00Z"`,
		`"healthy": true`,
		`"unbalancedEntries": []`,
		`"mismatches": []`,
	} {
		if !strings.Contains(stdout.String(), fragment) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), fragment)
		}
	}
}

func TestRunReturnsInconsistentExitCode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := run(
		context.Background(),
		mapLookup(map[string]string{
			"GOCHYA_DATABASE_URL": "postgres://gochya@db/gochya",
		}),
		&stdout,
		&stderr,
		func(
			_ context.Context,
			_ string,
		) (ledgeraudit.Report, error) {
			return ledgeraudit.Report{
				Healthy: false,
				Mismatches: []ledgeraudit.Mismatch{{
					Kind:     ledgeraudit.KindCurrency,
					PlayerID: "00000000-0000-0000-0000-000000000001",
					Asset:    "koins",
				}},
				UnbalancedEntries: []ledgeraudit.UnbalancedEntry{},
			}, nil
		},
	)
	if exitCode != exitInconsistent {
		t.Fatalf("exit code = %d", exitCode)
	}
	if stderr.Len() != 0 || !strings.Contains(stdout.String(), `"healthy": false`) {
		t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestRunRejectsMissingDatabaseURL(t *testing.T) {
	var stdout, stderr bytes.Buffer
	called := false
	exitCode := run(
		context.Background(),
		mapLookup(map[string]string{}),
		&stdout,
		&stderr,
		func(
			_ context.Context,
			_ string,
		) (ledgeraudit.Report, error) {
			called = true
			return ledgeraudit.Report{}, nil
		},
	)
	if exitCode != exitRuntimeFailure || called || stdout.Len() != 0 {
		t.Fatalf(
			"exit code = %d, called = %v, stdout = %q",
			exitCode,
			called,
			stdout.String(),
		)
	}
	if !strings.Contains(stderr.String(), "GOCHYA_DATABASE_URL is required") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunReportsRuntimeFailure(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := run(
		context.Background(),
		mapLookup(map[string]string{
			"GOCHYA_DATABASE_URL": "postgres://gochya@db/gochya",
		}),
		&stdout,
		&stderr,
		func(
			_ context.Context,
			_ string,
		) (ledgeraudit.Report, error) {
			return ledgeraudit.Report{}, errors.New("database unavailable")
		},
	)
	if exitCode != exitRuntimeFailure || stdout.Len() != 0 {
		t.Fatalf("exit code = %d, stdout = %q", exitCode, stdout.String())
	}
	if !strings.Contains(stderr.String(), "database unavailable") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func mapLookup(values map[string]string) environmentLookup {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}
