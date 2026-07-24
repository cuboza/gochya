package profile

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const maxPlayerIDLength = 128

type ServiceConfig struct {
	Store Store
	Now   func() time.Time
}

type Service struct {
	store Store
	now   func() time.Time
}

func NewService(config ServiceConfig) (*Service, error) {
	if config.Store == nil {
		return nil, errors.New("profile store is required")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &Service{store: config.Store, now: config.Now}, nil
}

func (s *Service) PlayerProfile(
	ctx context.Context,
	playerID string,
) (PlayerProfile, error) {
	if err := validatePlayerID(playerID); err != nil {
		return PlayerProfile{}, err
	}
	response, err := s.store.PlayerProfile(ctx, playerID)
	if errors.Is(err, ErrPlayerNotFound) {
		return PlayerProfile{}, apiError(
			"profile_not_found",
			"player profile was not found",
			http.StatusNotFound,
		)
	}
	if err != nil {
		return PlayerProfile{}, asAPIError(fmt.Errorf("load player profile: %w", err))
	}
	return response, nil
}

func (s *Service) ListPets(ctx context.Context, playerID string) ([]Pet, error) {
	if err := validatePlayerID(playerID); err != nil {
		return nil, err
	}
	pets, err := s.store.ListPets(ctx, playerID)
	if err != nil {
		return nil, asAPIError(fmt.Errorf("list pets: %w", err))
	}
	if pets == nil {
		pets = []Pet{}
	}
	return pets, nil
}

func (s *Service) Pet(
	ctx context.Context,
	playerID string,
	petID string,
) (Pet, error) {
	if err := validatePlayerID(playerID); err != nil {
		return Pet{}, err
	}
	if err := validatePetID(petID); err != nil {
		return Pet{}, err
	}
	pet, err := s.store.Pet(ctx, playerID, petID)
	if errors.Is(err, ErrPetNotFound) {
		return Pet{}, petNotFoundError()
	}
	if err != nil {
		return Pet{}, asAPIError(fmt.Errorf("load pet: %w", err))
	}
	return pet, nil
}

func (s *Service) Lineage(
	ctx context.Context,
	playerID string,
	petID string,
) (LineageTree, error) {
	if err := validatePlayerID(playerID); err != nil {
		return LineageTree{}, err
	}
	if err := validatePetID(petID); err != nil {
		return LineageTree{}, err
	}
	lineage, err := s.store.Lineage(ctx, playerID, petID)
	if errors.Is(err, ErrPetNotFound) {
		return LineageTree{}, petNotFoundError()
	}
	if err != nil {
		return LineageTree{}, asAPIError(
			fmt.Errorf("load pet lineage: %w", err),
		)
	}
	if lineage.Nodes == nil {
		lineage.Nodes = []LineageNode{}
	}
	return lineage, nil
}

func (s *Service) ActivatePet(
	ctx context.Context,
	playerID string,
	petID string,
) (Pet, error) {
	if err := validatePlayerID(playerID); err != nil {
		return Pet{}, err
	}
	if err := validatePetID(petID); err != nil {
		return Pet{}, err
	}
	pet, err := s.store.ActivatePet(ctx, ActivatePetCommit{
		PlayerID: playerID,
		PetID:    petID,
		Now:      s.now().UTC(),
	})
	switch {
	case errors.Is(err, ErrPlayerNotFound):
		return Pet{}, apiError(
			"profile_not_found",
			"player profile was not found",
			http.StatusNotFound,
		)
	case errors.Is(err, ErrPetNotFound):
		return Pet{}, petNotFoundError()
	case err != nil:
		return Pet{}, asAPIError(fmt.Errorf("activate pet: %w", err))
	default:
		return pet, nil
	}
}

func validatePlayerID(playerID string) error {
	if strings.TrimSpace(playerID) == "" || len(playerID) > maxPlayerIDLength {
		return apiError(
			"identity_invalid",
			"authenticated player is required",
			http.StatusBadRequest,
		)
	}
	return nil
}

func validatePetID(petID string) error {
	if !validUUID(petID) {
		return apiError(
			"pet_id_invalid",
			"pet ID must be a UUID",
			http.StatusBadRequest,
		)
	}
	return nil
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' ||
		value[18] != '-' || value[23] != '-' {
		return false
	}
	compact := strings.ReplaceAll(value, "-", "")
	decoded := make([]byte, 16)
	_, err := hex.Decode(decoded, []byte(compact))
	return err == nil
}

func petNotFoundError() *Error {
	return apiError(
		"pet_not_found",
		"pet was not found",
		http.StatusNotFound,
	)
}
