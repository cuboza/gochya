package battle

import (
	"context"
	"errors"
)

type Store interface {
	QueueCasual(context.Context, QueueCommit, Simulator) (QueueResponse, error)
	Match(context.Context, string, string) (MatchResponse, error)
}

var (
	ErrPlayerNotFound      = errors.New("player not found")
	ErrLoadoutRequired     = errors.New("complete loadout is required")
	ErrPetWeak             = errors.New("weak pet cannot fight")
	ErrNoOpponent          = errors.New("no eligible opponent")
	ErrMatchNotFound       = errors.New("match not found")
	ErrIdempotencyConflict = errors.New("idempotency key conflict")
)
