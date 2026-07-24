package onboarding

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
	"strings"
	"time"

	"github.com/gochya/gochya/server/internal/corebridge"
)

const (
	maxDeclaredAge = 120
	dateLayout     = "2006-01-02"
)

type ServiceConfig struct {
	Store  Store
	Core   corebridge.StarterEngine
	Now    func() time.Time
	Random io.Reader
}

type Service struct {
	store  Store
	core   corebridge.StarterEngine
	now    func() time.Time
	random io.Reader
}

func NewService(config ServiceConfig) (*Service, error) {
	if config.Store == nil || config.Core == nil {
		return nil, errors.New("onboarding store and core are required")
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

func (s *Service) RecordAgeGate(
	ctx context.Context,
	playerID string,
	idempotencyKey string,
	request AgeGateRequest,
) (AgeGateResponse, error) {
	if strings.TrimSpace(playerID) == "" {
		return AgeGateResponse{}, apiError(
			"identity_invalid",
			"authenticated player is required",
			http.StatusBadRequest,
		)
	}
	if !validUUID(idempotencyKey) {
		return AgeGateResponse{}, apiError(
			"idempotency_key_invalid",
			"Idempotency-Key must be a UUID",
			http.StatusBadRequest,
		)
	}
	now := s.now().UTC()
	birthDate, err := parseBirthDate(request.BirthDate, now)
	if err != nil {
		return AgeGateResponse{}, err
	}
	ageBand := AgeBand13Plus
	if ageOnDate(birthDate, now) < 13 {
		ageBand = AgeBandUnder13
	}
	response, err := s.store.RecordAgeGate(ctx, AgeGateCommit{
		PlayerID:       playerID,
		AgeBand:        ageBand,
		IdempotencyKey: strings.ToLower(idempotencyKey),
		Now:            now,
	})
	return response, mapStoreError(err)
}

func (s *Service) SelectStarterEgg(
	ctx context.Context,
	playerID string,
	idempotencyKey string,
	request StarterEggRequest,
) (StarterEggResponse, error) {
	if strings.TrimSpace(playerID) == "" {
		return StarterEggResponse{}, apiError(
			"identity_invalid",
			"authenticated player is required",
			http.StatusBadRequest,
		)
	}
	if !validUUID(idempotencyKey) {
		return StarterEggResponse{}, apiError(
			"idempotency_key_invalid",
			"Idempotency-Key must be a UUID",
			http.StatusBadRequest,
		)
	}
	elementID, ok := starterElementID(request.Element)
	if !ok {
		return StarterEggResponse{}, apiError(
			"starter_element_invalid",
			"element must be fire, water, or earth",
			http.StatusBadRequest,
		)
	}
	canonical, err := json.Marshal(struct {
		Element string `json:"element"`
	}{Element: request.Element})
	if err != nil {
		return StarterEggResponse{}, asAPIError(err)
	}
	eggID, err := randomUUID(s.random)
	if err != nil {
		return StarterEggResponse{}, asAPIError(
			fmt.Errorf("generate starter egg ID: %w", err),
		)
	}
	var seedBytes [8]byte
	if _, err := io.ReadFull(s.random, seedBytes[:]); err != nil {
		return StarterEggResponse{}, asAPIError(
			fmt.Errorf("generate starter seed: %w", err),
		)
	}
	response, err := s.store.SelectStarterEgg(ctx, StarterEggCommit{
		PlayerID:       playerID,
		Element:        request.Element,
		ElementID:      elementID,
		IdempotencyKey: strings.ToLower(idempotencyKey),
		RequestHash:    sha256.Sum256(canonical),
		EggID:          eggID,
		Seed:           binary.BigEndian.Uint64(seedBytes[:]),
		Now:            s.now().UTC(),
	}, s.core)
	return response, mapStoreError(err)
}

func parseBirthDate(value string, now time.Time) (time.Time, error) {
	birthDate, err := time.Parse(dateLayout, value)
	if err != nil || birthDate.Format(dateLayout) != value {
		return time.Time{}, apiError(
			"birth_date_invalid",
			"birthDate must be a real date in YYYY-MM-DD format",
			http.StatusBadRequest,
		)
	}
	today := time.Date(
		now.Year(),
		now.Month(),
		now.Day(),
		0,
		0,
		0,
		0,
		time.UTC,
	)
	if birthDate.After(today) || ageOnDate(birthDate, today) > maxDeclaredAge {
		return time.Time{}, apiError(
			"birth_date_invalid",
			"birthDate is outside the supported range",
			http.StatusBadRequest,
		)
	}
	return birthDate, nil
}

func ageOnDate(birthDate time.Time, date time.Time) int {
	age := date.Year() - birthDate.Year()
	if date.Month() < birthDate.Month() ||
		(date.Month() == birthDate.Month() && date.Day() < birthDate.Day()) {
		age--
	}
	return age
}

func starterElementID(value string) (uint8, bool) {
	switch value {
	case StarterElementFire:
		return 0, true
	case StarterElementWater:
		return 1, true
	case StarterElementEarth:
		return 2, true
	default:
		return 0, false
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
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(value)
	return encoded[0:8] + "-" +
		encoded[8:12] + "-" +
		encoded[12:16] + "-" +
		encoded[16:20] + "-" +
		encoded[20:32], nil
}
