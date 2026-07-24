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
		confirmation: ConfirmResponse{
			MatchID: "22222222-2222-4222-8222-222222222222",
			Outcome: "win",
			Rewards: []Reward{{Currency: "koins", Amount: casualWinKoins}},
		},
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
	confirmation, err := service.Confirm(context.Background(), battlePlayer, response.MatchID)
	if err != nil || confirmation.MatchID != response.MatchID ||
		store.confirmCommit.PlayerID != battlePlayer {
		t.Fatalf("Confirm = %#v, %v, commit = %#v", confirmation, err, store.confirmCommit)
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

func TestServiceRejectsInvalidConfirm(t *testing.T) {
	service, _ := NewService(ServiceConfig{Store: &fakeStore{}, Core: fakeCore{}})
	_, err := service.Confirm(context.Background(), battlePlayer, "bad")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "match_id_invalid" {
		t.Fatalf("error = %v", err)
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

func TestCasualRewardUsesParticipantPerspective(t *testing.T) {
	tests := []struct {
		winner    string
		playerIsA bool
		outcome   string
		reward    int
	}{
		{"a", true, "win", casualWinKoins},
		{"a", false, "loss", casualLossKoins},
		{"b", true, "loss", casualLossKoins},
		{"b", false, "win", casualWinKoins},
		{"draw", true, "draw", casualDrawKoins},
		{"draw", false, "draw", casualDrawKoins},
	}
	for _, test := range tests {
		outcome, reward, err := casualReward(test.winner, test.playerIsA)
		if err != nil || outcome != test.outcome || reward != test.reward {
			t.Fatalf(
				"casualReward(%q, %t) = %q, %d, %v",
				test.winner,
				test.playerIsA,
				outcome,
				reward,
				err,
			)
		}
	}
	if _, _, err := casualReward("client-wins", true); err == nil {
		t.Fatal("invalid winner was accepted")
	}
}

type fakeStore struct {
	queue         QueueResponse
	match         MatchResponse
	history       []MatchSummary
	confirmation  ConfirmResponse
	commit        QueueCommit
	confirmCommit ConfirmCommit
	historyLimit  int
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
func (s *fakeStore) Confirm(
	_ context.Context,
	input ConfirmCommit,
) (ConfirmResponse, error) {
	s.confirmCommit = input
	return s.confirmation, nil
}

type fakeCore struct{}

func (fakeCore) SimulateCombat(
	context.Context,
	corebridge.CombatMatch,
	uint64,
) (corebridge.CombatResult, error) {
	return corebridge.CombatResult{}, nil
}
