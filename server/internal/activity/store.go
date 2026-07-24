package activity

import (
	"context"
	"errors"

	"github.com/gochya/gochya/server/internal/corebridge"
)

type Store interface {
	Sync(
		context.Context,
		SyncCommit,
		corebridge.ActivityEngine,
	) (SyncResponse, error)
}

var (
	ErrPlayerNotFound    = errors.New("player not found")
	ErrActivePetRequired = errors.New("active pet is required")
	ErrSnapshotDate      = errors.New("snapshot must belong to the current player day")
	ErrPetStateInvalid   = errors.New("active pet state is invalid")
)
