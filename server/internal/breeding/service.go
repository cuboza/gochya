package breeding

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/gochya/gochya/server/internal/corebridge"
)

type ServiceConfig struct {
	Store  Store
	Core   corebridge.BreedingEngine
	Now    func() time.Time
	Random io.Reader
}

type Service struct {
	store  Store
	core   corebridge.BreedingEngine
	now    func() time.Time
	random io.Reader
}

func NewService(config ServiceConfig) (*Service, error) {
	if config.Store == nil || config.Core == nil {
		return nil, errors.New("breeding store and core are required")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.Random == nil {
		config.Random = rand.Reader
	}
	return &Service{
		store:  config.Store,
		core:   config.Core,
		now:    config.Now,
		random: config.Random,
	}, nil
}

func (s *Service) Breed(
	ctx context.Context,
	playerID string,
	idempotencyKey string,
	request BreedRequest,
) (BreedResponse, error) {
	if strings.TrimSpace(playerID) == "" {
		return BreedResponse{}, apiError(
			"identity_invalid",
			"authenticated player is required",
			http.StatusBadRequest,
		)
	}
	if !validUUID(idempotencyKey) {
		return BreedResponse{}, apiError(
			"idempotency_key_invalid",
			"Idempotency-Key must be a UUID",
			http.StatusBadRequest,
		)
	}
	if !validUUID(request.ParentAID) || !validUUID(request.ParentBID) {
		return BreedResponse{}, apiError(
			"parent_id_invalid",
			"parentA and parentB must be UUIDs",
			http.StatusBadRequest,
		)
	}
	parentAID := strings.ToLower(request.ParentAID)
	parentBID := strings.ToLower(request.ParentBID)
	if parentAID == parentBID {
		return BreedResponse{}, mapStoreError(ErrParentsIdentical)
	}
	mutation, hybrid, err := parseCatalysts(request.Catalysts)
	if err != nil {
		return BreedResponse{}, err
	}
	canonical, err := json.Marshal(struct {
		ParentAID string `json:"parentA"`
		ParentBID string `json:"parentB"`
		Mutation  bool   `json:"mutation"`
		Hybrid    bool   `json:"hybrid"`
	}{
		ParentAID: parentAID,
		ParentBID: parentBID,
		Mutation:  mutation,
		Hybrid:    hybrid,
	})
	if err != nil {
		return BreedResponse{}, asAPIError(err)
	}
	eggID, err := randomUUID(s.random)
	if err != nil {
		return BreedResponse{}, asAPIError(fmt.Errorf("generate egg ID: %w", err))
	}
	var seedBytes [8]byte
	if _, err := io.ReadFull(s.random, seedBytes[:]); err != nil {
		return BreedResponse{}, asAPIError(fmt.Errorf("generate breeding seed: %w", err))
	}
	response, err := s.store.Breed(ctx, BreedCommit{
		PlayerID:         playerID,
		ParentAID:        parentAID,
		ParentBID:        parentBID,
		MutationCatalyst: mutation,
		HybridCatalyst:   hybrid,
		IdempotencyKey:   strings.ToLower(idempotencyKey),
		RequestHash:      sha256.Sum256(canonical),
		EggID:            eggID,
		Seed:             binary.BigEndian.Uint64(seedBytes[:]),
		Now:              s.now().UTC(),
	}, s.core)
	return response, mapStoreError(err)
}

func (s *Service) ListEggs(
	ctx context.Context,
	playerID string,
) ([]Egg, error) {
	if strings.TrimSpace(playerID) == "" {
		return nil, apiError(
			"identity_invalid",
			"authenticated player is required",
			http.StatusBadRequest,
		)
	}
	eggs, err := s.store.ListEggs(ctx, playerID)
	if err != nil {
		return nil, mapStoreError(err)
	}
	if eggs == nil {
		eggs = []Egg{}
	}
	return eggs, nil
}

func (s *Service) Hatch(
	ctx context.Context,
	playerID string,
	eggID string,
) (Pet, error) {
	if strings.TrimSpace(playerID) == "" {
		return Pet{}, apiError(
			"identity_invalid",
			"authenticated player is required",
			http.StatusBadRequest,
		)
	}
	if !validUUID(eggID) {
		return Pet{}, apiError(
			"egg_id_invalid",
			"egg ID must be a UUID",
			http.StatusBadRequest,
		)
	}
	petID, err := randomUUID(s.random)
	if err != nil {
		return Pet{}, asAPIError(fmt.Errorf("generate pet ID: %w", err))
	}
	pet, err := s.store.Hatch(ctx, HatchCommit{
		PlayerID: playerID,
		EggID:    eggID,
		PetID:    petID,
		Now:      s.now().UTC(),
	})
	return pet, mapStoreError(err)
}

func parseCatalysts(values []string) (bool, bool, error) {
	if len(values) > 2 {
		return false, false, apiError(
			"catalysts_invalid",
			"catalysts may contain mutation and hybrid at most once",
			http.StatusBadRequest,
		)
	}
	if slices.Contains(values, "") {
		return false, false, apiError(
			"catalysts_invalid",
			"catalyst names must not be empty",
			http.StatusBadRequest,
		)
	}
	mutation := false
	hybrid := false
	for _, catalyst := range values {
		switch catalyst {
		case "mutation":
			if mutation {
				return false, false, apiError(
					"catalysts_invalid",
					"catalysts must be unique",
					http.StatusBadRequest,
				)
			}
			mutation = true
		case "hybrid":
			if hybrid {
				return false, false, apiError(
					"catalysts_invalid",
					"catalysts must be unique",
					http.StatusBadRequest,
				)
			}
			hybrid = true
		default:
			return false, false, apiError(
				"catalysts_invalid",
				"supported catalysts are mutation and hybrid",
				http.StatusBadRequest,
			)
		}
	}
	return mutation, hybrid, nil
}

func mapStoreError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrPlayerNotFound):
		return apiError("profile_not_found", "player profile was not found", http.StatusNotFound)
	case errors.Is(err, ErrParentNotFound):
		return apiError("parent_not_found", "breeding parent was not found", http.StatusNotFound)
	case errors.Is(err, ErrParentsIdentical):
		return apiError("parents_identical", "two distinct parents are required", http.StatusBadRequest)
	case errors.Is(err, ErrParentStateInvalid):
		return apiError("parent_ineligible", "parents must be healthy level 30 adults", http.StatusConflict)
	case errors.Is(err, ErrParentsTooRelated):
		return apiError("parents_too_related", "parents are too closely related", http.StatusConflict)
	case errors.Is(err, ErrParentCooldown):
		return apiError("parent_cooldown", "a parent is still on breeding cooldown", http.StatusConflict)
	case errors.Is(err, ErrInsufficientKoins):
		return apiError("insufficient_koins", "500 Koins are required", http.StatusConflict)
	case errors.Is(err, ErrLoveCrystalRequired):
		return apiError("love_crystal_required", "one Love Crystal is required", http.StatusConflict)
	case errors.Is(err, ErrCatalystRequired):
		return apiError("catalyst_required", "a requested catalyst is unavailable", http.StatusConflict)
	case errors.Is(err, ErrIdempotencyConflict):
		return apiError(
			"idempotency_conflict",
			"Idempotency-Key was already used with another breeding request",
			http.StatusConflict,
		)
	case errors.Is(err, ErrEggNotFound):
		return apiError("egg_not_found", "egg was not found", http.StatusNotFound)
	case errors.Is(err, ErrEggNotReady):
		return apiError("egg_not_ready", "egg is still incubating", http.StatusConflict)
	case errors.Is(err, ErrGenomeInvalid):
		return apiError("genome_invalid", "stored genome is invalid", http.StatusConflict)
	case errors.Is(err, corebridge.ErrUnavailable):
		return apiError("core_unavailable", "Gochya Core is unavailable", http.StatusServiceUnavailable)
	default:
		return asAPIError(err)
	}
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' ||
		value[18] != '-' || value[23] != '-' {
		return false
	}
	decoded := make([]byte, 16)
	_, err := hex.Decode(decoded, []byte(strings.ReplaceAll(value, "-", "")))
	return err == nil
}

func randomUUID(reader io.Reader) (string, error) {
	value := make([]byte, 16)
	if _, err := io.ReadFull(reader, value); err != nil {
		return "", err
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	return fmt.Sprintf(
		"%x-%x-%x-%x-%x",
		value[:4],
		value[4:6],
		value[6:8],
		value[8:10],
		value[10:],
	), nil
}
