package profile

import (
	"context"
	"errors"
)

type Store interface {
	PlayerProfile(context.Context, string) (PlayerProfile, error)
	ListPets(context.Context, string) ([]Pet, error)
	Pet(context.Context, string, string) (Pet, error)
	Lineage(context.Context, string, string) (LineageTree, error)
	ActivatePet(context.Context, ActivatePetCommit) (Pet, error)
}

var (
	ErrPlayerNotFound = errors.New("player not found")
	ErrPetNotFound    = errors.New("pet not found")
	ErrLineageInvalid = errors.New("pet lineage is invalid")
)
