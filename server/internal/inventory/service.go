package inventory

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	defaultPageLimit = 20
	maxPageLimit     = 100
	maxCursorLength  = 512
	maxPlayerIDLen   = 128
)

type Service struct {
	store TechniqueStore
}

func NewService(store TechniqueStore) (*Service, error) {
	if store == nil {
		return nil, errors.New("technique store is required")
	}
	return &Service{store: store}, nil
}

func (s *Service) ListTechniques(
	ctx context.Context,
	playerID string,
	limitValue string,
	cursorValue string,
) (ListTechniquesResponse, error) {
	if strings.TrimSpace(playerID) == "" || len(playerID) > maxPlayerIDLen {
		return ListTechniquesResponse{}, apiError(
			"identity_invalid",
			"authenticated player is required",
			http.StatusBadRequest,
		)
	}
	limit, err := parseLimit(limitValue)
	if err != nil {
		return ListTechniquesResponse{}, err
	}
	cursor, err := decodeCursor(cursorValue)
	if err != nil {
		return ListTechniquesResponse{}, err
	}
	cards, err := s.store.ListTechniqueCards(ctx, playerID, cursor, limit+1)
	if err != nil {
		return ListTechniquesResponse{}, asAPIError(
			fmt.Errorf("list technique cards: %w", err),
		)
	}
	hasMore := len(cards) > limit
	if hasMore {
		cards = cards[:limit]
	}
	response := ListTechniquesResponse{Items: cards}
	if hasMore && len(cards) > 0 {
		response.NextCursor, err = encodeCursor(TechniqueCursor{
			CreatedAt: cards[len(cards)-1].CreatedAt,
			ID:        cards[len(cards)-1].ID,
		})
		if err != nil {
			return ListTechniquesResponse{}, asAPIError(
				fmt.Errorf("encode technique cursor: %w", err),
			)
		}
	}
	return response, nil
}

func (s *Service) EquipTechniques(
	ctx context.Context,
	playerID string,
	idempotencyKey string,
	request EquipTechniquesRequest,
) (LoadoutResponse, error) {
	if err := validatePlayerID(playerID); err != nil {
		return LoadoutResponse{}, err
	}
	if validateUUID(idempotencyKey) != nil {
		return LoadoutResponse{}, apiError(
			"idempotency_key_invalid",
			"Idempotency-Key must be a UUID",
			http.StatusBadRequest,
		)
	}
	if len(request.CardIDs) != 5 || request.SignatureIdx > 4 {
		return LoadoutResponse{}, invalidLoadoutError()
	}
	seen := make(map[string]struct{}, len(request.CardIDs))
	for _, cardID := range request.CardIDs {
		if validateUUID(cardID) != nil {
			return LoadoutResponse{}, invalidLoadoutError()
		}
		if _, exists := seen[cardID]; exists {
			return LoadoutResponse{}, invalidLoadoutError()
		}
		seen[cardID] = struct{}{}
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return LoadoutResponse{}, asAPIError(
			fmt.Errorf("encode equip request: %w", err),
		)
	}
	response, err := s.store.EquipTechniques(ctx, EquipCommit{
		PlayerID:       playerID,
		CardIDs:        append([]string(nil), request.CardIDs...),
		SignatureIdx:   request.SignatureIdx,
		IdempotencyKey: idempotencyKey,
		RequestHash:    sha256.Sum256(encoded),
		Now:            time.Now().UTC(),
	})
	switch {
	case errors.Is(err, ErrActivePetRequired):
		return LoadoutResponse{}, apiError(
			"active_pet_required",
			"an active pet is required to build a loadout",
			http.StatusConflict,
		)
	case errors.Is(err, ErrLoadoutCardsInvalid):
		return LoadoutResponse{}, apiError(
			"loadout_cards_invalid",
			"loadout must contain five owned technique cards",
			http.StatusConflict,
		)
	case errors.Is(err, ErrIdempotencyConflict):
		return LoadoutResponse{}, apiError(
			"idempotency_conflict",
			"Idempotency-Key was already used with another request",
			http.StatusConflict,
		)
	case err != nil:
		return LoadoutResponse{}, asAPIError(fmt.Errorf("equip techniques: %w", err))
	default:
		return response, nil
	}
}

func (s *Service) CurrentLoadout(
	ctx context.Context,
	playerID string,
) (LoadoutResponse, error) {
	if err := validatePlayerID(playerID); err != nil {
		return LoadoutResponse{}, err
	}
	response, err := s.store.CurrentLoadout(ctx, playerID)
	if errors.Is(err, ErrLoadoutNotFound) {
		return LoadoutResponse{}, apiError(
			"loadout_not_found",
			"player does not have a loadout",
			http.StatusNotFound,
		)
	}
	if err != nil {
		return LoadoutResponse{}, asAPIError(fmt.Errorf("load current loadout: %w", err))
	}
	return response, nil
}

func validatePlayerID(playerID string) error {
	if strings.TrimSpace(playerID) == "" || len(playerID) > maxPlayerIDLen {
		return apiError(
			"identity_invalid",
			"authenticated player is required",
			http.StatusBadRequest,
		)
	}
	return nil
}

func invalidLoadoutError() *Error {
	return apiError(
		"loadout_invalid",
		"loadout requires five distinct card UUIDs and signatureIdx from 0 to 4",
		http.StatusBadRequest,
	)
}

func parseLimit(value string) (int, error) {
	if value == "" {
		return defaultPageLimit, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 1 || limit > maxPageLimit {
		return 0, apiError(
			"invalid_limit",
			"limit must be an integer between 1 and 100",
			http.StatusBadRequest,
		)
	}
	return limit, nil
}

type cursorPayload struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

func encodeCursor(cursor TechniqueCursor) (string, error) {
	encoded, err := json.Marshal(cursorPayload{
		CreatedAt: cursor.CreatedAt.UTC(),
		ID:        cursor.ID,
	})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeCursor(value string) (*TechniqueCursor, error) {
	if value == "" {
		return nil, nil
	}
	if len(value) > maxCursorLength {
		return nil, invalidCursorError()
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || base64.RawURLEncoding.EncodeToString(decoded) != value {
		return nil, invalidCursorError()
	}
	decoder := json.NewDecoder(bytes.NewReader(decoded))
	decoder.DisallowUnknownFields()
	var payload cursorPayload
	if err := decoder.Decode(&payload); err != nil {
		return nil, invalidCursorError()
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, invalidCursorError()
	}
	if payload.CreatedAt.IsZero() || validateUUID(payload.ID) != nil {
		return nil, invalidCursorError()
	}
	return &TechniqueCursor{
		CreatedAt: payload.CreatedAt.UTC(),
		ID:        payload.ID,
	}, nil
}

func validateUUID(value string) error {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' ||
		value[18] != '-' || value[23] != '-' {
		return errors.New("invalid UUID")
	}
	compact := strings.ReplaceAll(value, "-", "")
	decoded := make([]byte, 16)
	_, err := hex.Decode(decoded, []byte(compact))
	return err
}

func invalidCursorError() *Error {
	return apiError(
		"invalid_cursor",
		"cursor is invalid",
		http.StatusBadRequest,
	)
}
