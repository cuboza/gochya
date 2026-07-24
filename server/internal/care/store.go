package care

import (
	"context"

	"github.com/gochya/gochya/server/internal/corebridge"
)

type Store interface {
	Reconcile(context.Context, SyncCommit, corebridge.CareEngine) (SyncResponse, error)
}
