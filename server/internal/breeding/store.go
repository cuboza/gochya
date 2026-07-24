package breeding

import (
	"context"

	"github.com/gochya/gochya/server/internal/corebridge"
)

type Store interface {
	Breed(context.Context, BreedCommit, corebridge.BreedingEngine) (BreedResponse, error)
	ListEggs(context.Context, string) ([]Egg, error)
	Hatch(context.Context, HatchCommit) (Pet, error)
}
