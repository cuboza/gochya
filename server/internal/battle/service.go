package battle

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gochya/gochya/server/internal/corebridge"
)

const (
	defaultHistoryLimit = 20
	maxHistoryLimit     = 100
)

type ServiceConfig struct {
	Store  Store
	Core   corebridge.CombatEngine
	Now    func() time.Time
	Random io.Reader
}

type Service struct {
	store  Store
	core   corebridge.CombatEngine
	now    func() time.Time
	random io.Reader
}

func NewService(config ServiceConfig) (*Service, error) {
	if config.Store == nil || config.Core == nil {
		return nil, errors.New("battle store and combat core are required")
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

func (s *Service) Queue(
	ctx context.Context,
	playerID string,
	idempotencyKey string,
	request QueueRequest,
) (QueueResponse, error) {
	if strings.TrimSpace(playerID) == "" {
		return QueueResponse{}, apiError(
			"identity_invalid",
			"authenticated player is required",
			http.StatusBadRequest,
		)
	}
	if !validUUID(idempotencyKey) {
		return QueueResponse{}, apiError(
			"idempotency_key_invalid",
			"Idempotency-Key must be a UUID",
			http.StatusBadRequest,
		)
	}
	if request.Mode != "casual" {
		return QueueResponse{}, apiError(
			"mode_invalid",
			"only casual mode is currently available",
			http.StatusBadRequest,
		)
	}
	body, err := json.Marshal(request)
	if err != nil {
		return QueueResponse{}, asAPIError(err)
	}
	matchID, err := randomUUID(s.random)
	if err != nil {
		return QueueResponse{}, asAPIError(fmt.Errorf("generate match ID: %w", err))
	}
	seedBytes := make([]byte, 8)
	if _, err := io.ReadFull(s.random, seedBytes); err != nil {
		return QueueResponse{}, asAPIError(fmt.Errorf("generate match seed: %w", err))
	}
	seed := uint64(0)
	for _, value := range seedBytes {
		seed = seed<<8 | uint64(value)
	}
	seed &= uint64(^uint64(0) >> 1)
	response, err := s.store.QueueCasual(ctx, QueueCommit{
		PlayerID:       playerID,
		IdempotencyKey: idempotencyKey,
		RequestHash:    sha256.Sum256(body),
		MatchID:        matchID,
		Seed:           seed,
		Now:            s.now().UTC(),
	}, s.core)
	return response, mapStoreError(err)
}

func (s *Service) Match(
	ctx context.Context,
	playerID string,
	matchID string,
) (MatchResponse, error) {
	if strings.TrimSpace(playerID) == "" {
		return MatchResponse{}, apiError(
			"identity_invalid",
			"authenticated player is required",
			http.StatusBadRequest,
		)
	}
	if !validUUID(matchID) {
		return MatchResponse{}, apiError(
			"match_id_invalid",
			"match ID must be a UUID",
			http.StatusBadRequest,
		)
	}
	response, err := s.store.Match(ctx, playerID, matchID)
	return response, mapStoreError(err)
}

func (s *Service) History(
	ctx context.Context,
	playerID string,
	rawLimit string,
) ([]MatchSummary, error) {
	if strings.TrimSpace(playerID) == "" {
		return nil, apiError(
			"identity_invalid",
			"authenticated player is required",
			http.StatusBadRequest,
		)
	}
	limit := defaultHistoryLimit
	if rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 1 || parsed > maxHistoryLimit {
			return nil, apiError(
				"limit_invalid",
				"limit must be an integer between 1 and 100",
				http.StatusBadRequest,
			)
		}
		limit = parsed
	}
	response, err := s.store.History(ctx, playerID, limit)
	if err != nil {
		return nil, mapStoreError(err)
	}
	if response == nil {
		response = []MatchSummary{}
	}
	return response, nil
}

func mapStoreError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrPlayerNotFound):
		return apiError("profile_not_found", "player profile was not found", http.StatusNotFound)
	case errors.Is(err, ErrLoadoutRequired):
		return apiError("loadout_required", "complete loadout is required", http.StatusConflict)
	case errors.Is(err, ErrPetWeak):
		return apiError("pet_weak", "weak pet cannot enter combat", http.StatusConflict)
	case errors.Is(err, ErrNoOpponent):
		return apiError("no_opponent", "no eligible casual opponent is available", http.StatusConflict)
	case errors.Is(err, ErrMatchNotFound):
		return apiError("match_not_found", "match was not found", http.StatusNotFound)
	case errors.Is(err, ErrIdempotencyConflict):
		return apiError(
			"idempotency_conflict",
			"Idempotency-Key was already used with another request",
			http.StatusConflict,
		)
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
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[:4], value[4:6], value[6:8], value[8:10], value[10:]), nil
}
