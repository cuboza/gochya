package ledgeraudit

import (
	"testing"
	"time"
)

func TestReportFinalize(t *testing.T) {
	report := Report{
		CheckedAt:         time.Date(2026, time.July, 24, 14, 0, 0, 0, time.UTC),
		UnbalancedEntries: []UnbalancedEntry{},
		Mismatches:        []Mismatch{},
	}
	report.finalize()
	if !report.Healthy {
		t.Fatalf("report = %#v", report)
	}
	report.UnbalancedEntries = append(report.UnbalancedEntries, UnbalancedEntry{})
	report.finalize()
	if report.Healthy {
		t.Fatal("report stayed healthy with an unbalanced entry")
	}
	report.UnbalancedEntries = report.UnbalancedEntries[:0]
	report.Mismatches = append(report.Mismatches, Mismatch{})
	report.finalize()
	if report.Healthy {
		t.Fatal("report stayed healthy with a projection mismatch")
	}
}
