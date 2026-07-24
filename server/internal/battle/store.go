package battle

import (
	"context"
	"errors"

	"github.com/gochya/gochya/server/internal/corebridge"
)

type Store interface {
	QueueCasual(context.Context, QueueCommit, Simulator) (QueueResponse, error)
	Match(context.Context, string, string) (MatchResponse, error)
	History(context.Context, string, int) ([]MatchSummary, error)
	Confirm(context.Context, ConfirmCommit, corebridge.LootEngine) (ConfirmResponse, error)
}

var (
	ErrPlayerNotFound      = errors.New("player not found")
	ErrLoadoutRequired     = errors.New("complete loadout is required")
	ErrPetWeak             = errors.New("weak pet cannot fight")
	ErrNoOpponent          = errors.New("no eligible opponent")
	ErrMatchNotFound       = errors.New("match not found")
	ErrIdempotencyConflict = errors.New("idempotency key conflict")
)
