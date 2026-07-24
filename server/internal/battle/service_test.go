package battle

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/gochya/gochya/server/internal/corebridge"
)

const battlePlayer = "11111111-1111-4111-8111-111111111111"

func TestServiceQueuesAndReadsMatch(t *testing.T) {
	store := &fakeStore{
		queue: QueueResponse{MatchID: "22222222-2222-4222-8222-222222222222", Status: "completed"},
		match: MatchResponse{ID: "22222222-2222-4222-8222-222222222222"},
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

type fakeStore struct {
	queue  QueueResponse
	match  MatchResponse
	commit QueueCommit
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

type fakeCore struct{}

func (fakeCore) SimulateCombat(
	context.Context,
	corebridge.CombatMatch,
	uint64,
) (corebridge.CombatResult, error) {
	return corebridge.CombatResult{}, nil
}
