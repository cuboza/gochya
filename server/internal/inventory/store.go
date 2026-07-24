package inventory

import (
	"context"
	"errors"

	"github.com/gochya/gochya/server/internal/dojo"
)

type TechniqueStore interface {
	ListTechniqueCards(
		context.Context,
		string,
		*TechniqueCursor,
		int,
	) ([]dojo.TechniqueCard, error)
	EquipTechniques(context.Context, EquipCommit) (LoadoutResponse, error)
	CurrentLoadout(context.Context, string) (LoadoutResponse, error)
}

var (
	ErrActivePetRequired   = errors.New("active pet is required")
	ErrLoadoutCardsInvalid = errors.New("loadout cards are missing or not owned by player")
	ErrLoadoutNotFound     = errors.New("loadout not found")
	ErrIdempotencyConflict = errors.New("idempotency key reused with another equip request")
	ErrPlayerNotFound      = errors.New("player not found")
)
