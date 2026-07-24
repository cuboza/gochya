package battle

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gochya/gochya/server/internal/corebridge"
)

const battlePlayer = "11111111-1111-4111-8111-111111111111"

func TestServiceQueuesAndReadsMatch(t *testing.T) {
	store := &fakeStore{
		queue: QueueResponse{MatchID: "22222222-2222-4222-8222-222222222222", Status: "completed"},
		match: MatchResponse{ID: "22222222-2222-4222-8222-222222222222"},
		history: []MatchSummary{{
			ID:         "22222222-2222-4222-8222-222222222222",
			OpponentID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
			Outcome:    "win",
			CreatedAt:  time.Unix(1, 0).UTC(),
		}},
	}
	service, err := NewService(ServiceConfig{
		Store:  store,
		Core:   fakeCore{},
		Random: bytes.NewReader(make([]byte, 24)),
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	response, err := service.Queue(context.Background(), battlePlayer,
		"33333333-3333-4333-8333-333333333333", QueueRequest{Mode: "casual"})
	if err != nil || response.Status != "completed" {
		t.Fatalf("Queue = %#v, %v", response, err)
	}
	if store.commit.PlayerID != battlePlayer || store.commit.Seed != 0 ||
		store.commit.RequestHash == ([32]byte{}) {
		t.Fatalf("commit = %#v", store.commit)
	}
	match, err := service.Match(context.Background(), battlePlayer, response.MatchID)
	if err != nil || match.ID != response.MatchID {
		t.Fatalf("Match = %#v, %v", match, err)
	}
	history, err := service.History(context.Background(), battlePlayer, "")
	if err != nil || len(history) != 1 || store.historyLimit != defaultHistoryLimit {
		t.Fatalf("History = %#v, %v, limit = %d", history, err, store.historyLimit)
	}
}

func TestServiceRejectsInvalidQueue(t *testing.T) {
	service, _ := NewService(ServiceConfig{Store: &fakeStore{}, Core: fakeCore{}})
	for _, test := range []struct {
		key, mode, code string
	}{
		{"bad", "casual", "idempotency_key_invalid"},
		{"33333333-3333-4333-8333-333333333333", "ranked", "mode_invalid"},
	} {
		_, err := service.Queue(context.Background(), battlePlayer, test.key, QueueRequest{Mode: test.mode})
		var apiErr *Error
		if !errors.As(err, &apiErr) || apiErr.Code != test.code {
			t.Fatalf("error = %v, want %s", err, test.code)
		}
	}
}

func TestServiceValidatesHistoryLimit(t *testing.T) {
	store := &fakeStore{}
	service, _ := NewService(ServiceConfig{Store: store, Core: fakeCore{}})

	if _, err := service.History(context.Background(), battlePlayer, "100"); err != nil {
		t.Fatalf("History valid limit: %v", err)
	}
	if store.historyLimit != 100 {
		t.Fatalf("history limit = %d", store.historyLimit)
	}
	for _, raw := range []string{"0", "101", "-1", "one"} {
		_, err := service.History(context.Background(), battlePlayer, raw)
		var apiErr *Error
		if !errors.As(err, &apiErr) || apiErr.Code != "limit_invalid" {
			t.Fatalf("History(%q) error = %v", raw, err)
		}
	}
}

type fakeStore struct {
	queue        QueueResponse
	match        MatchResponse
	history      []MatchSummary
	commit       QueueCommit
	historyLimit int
}

func (s *fakeStore) QueueCasual(
	_ context.Context,
	input QueueCommit,
	_ Simulator,
) (QueueResponse, error) {
	s.commit = input
	return s.queue, nil
}
func (s *fakeStore) Match(context.Context, string, string) (MatchResponse, error) {
	return s.match, nil
}
func (s *fakeStore) History(_ context.Context, _ string, limit int) ([]MatchSummary, error) {
	s.historyLimit = limit
	return s.history, nil
}

type fakeCore struct{}

func (fakeCore) SimulateCombat(
	context.Context,
	corebridge.CombatMatch,
	uint64,
) (corebridge.CombatResult, error) {
	return corebridge.CombatResult{}, nil
}
