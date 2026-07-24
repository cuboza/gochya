package onboarding

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/gochya/gochya/server/internal/corebridge"
)

const (
	testPlayerID = "11111111-1111-4111-8111-111111111111"
	testEggID    = "22222222-2222-4222-8222-222222222222"
	testKey      = "33333333-3333-4333-8333-333333333333"
)

func TestServiceRecordsDerivedAgeBandWithoutBirthDate(t *testing.T) {
	now := time.Date(2026, time.July, 24, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{
		ageResponse: AgeGateResponse{
			Status:     AgeStatusEligible,
			RecordedAt: now,
		},
	}
	service := testService(t, store, now)

	response, err := service.RecordAgeGate(
		context.Background(),
		testPlayerID,
		testKey,
		AgeGateRequest{BirthDate: "2013-07-24"},
	)
	if err != nil {
		t.Fatalf("RecordAgeGate: %v", err)
	}
	if !reflect.DeepEqual(response, store.ageResponse) {
		t.Fatalf("response = %#v", response)
	}
	if store.ageCommit.PlayerID != testPlayerID ||
		store.ageCommit.AgeBand != AgeBand13Plus ||
		store.ageCommit.IdempotencyKey != testKey ||
		store.ageCommit.Now != now {
		t.Fatalf("age commit = %#v", store.ageCommit)
	}

	_, err = service.RecordAgeGate(
		context.Background(),
		testPlayerID,
		testKey,
		AgeGateRequest{BirthDate: "2013-07-25"},
	)
	if err != nil {
		t.Fatalf("RecordAgeGate under 13: %v", err)
	}
	if store.ageCommit.AgeBand != AgeBandUnder13 {
		t.Fatalf("age band = %q", store.ageCommit.AgeBand)
	}
}

func TestServiceRejectsInvalidAgeGateDeclarations(t *testing.T) {
	now := time.Date(2026, time.July, 24, 12, 0, 0, 0, time.UTC)
	service := testService(t, &fakeStore{}, now)
	tests := []struct {
		name      string
		player    string
		key       string
		birthDate string
		code      string
	}{
		{"identity", "", testKey, "2000-01-01", "identity_invalid"},
		{"key", testPlayerID, "bad", "2000-01-01", "idempotency_key_invalid"},
		{"empty date", testPlayerID, testKey, "", "birth_date_invalid"},
		{"noncanonical", testPlayerID, testKey, "2000-1-1", "birth_date_invalid"},
		{"impossible", testPlayerID, testKey, "2000-02-30", "birth_date_invalid"},
		{"future", testPlayerID, testKey, "2026-07-25", "birth_date_invalid"},
		{"implausible", testPlayerID, testKey, "1900-01-01", "birth_date_invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := service.RecordAgeGate(
				context.Background(),
				test.player,
				test.key,
				AgeGateRequest{BirthDate: test.birthDate},
			)
			var apiErr *Error
			if !errors.As(err, &apiErr) || apiErr.Code != test.code {
				t.Fatalf("error = %#v, want code %q", err, test.code)
			}
		})
	}
}

func TestServiceSelectsCanonicalStarterEgg(t *testing.T) {
	now := time.Date(2026, time.July, 24, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{
		starterResponse: StarterEggResponse{
			EggID:         testEggID,
			Element:       StarterElementWater,
			IncubateUntil: now.Add(starterIncubation),
		},
	}
	service := testService(t, store, now)
	response, err := service.SelectStarterEgg(
		context.Background(),
		testPlayerID,
		testKey,
		StarterEggRequest{Element: StarterElementWater},
	)
	if err != nil {
		t.Fatalf("SelectStarterEgg: %v", err)
	}
	if !reflect.DeepEqual(response, store.starterResponse) {
		t.Fatalf("response = %#v", response)
	}
	if store.starterCommit.PlayerID != testPlayerID ||
		store.starterCommit.Element != StarterElementWater ||
		store.starterCommit.ElementID != 1 ||
		store.starterCommit.IdempotencyKey != testKey ||
		store.starterCommit.RequestHash == ([32]byte{}) ||
		store.starterCommit.EggID == "" ||
		store.starterCommit.Now != now {
		t.Fatalf("starter commit = %#v", store.starterCommit)
	}

	for _, element := range []string{
		StarterElementFire,
		StarterElementWater,
		StarterElementEarth,
	} {
		if _, err := service.SelectStarterEgg(
			context.Background(),
			testPlayerID,
			testKey,
			StarterEggRequest{Element: element},
		); err != nil {
			t.Fatalf("SelectStarterEgg(%q): %v", element, err)
		}
	}
}

func TestServiceRejectsInvalidStarterSelectionAndMapsStoreErrors(t *testing.T) {
	now := time.Date(2026, time.July, 24, 12, 0, 0, 0, time.UTC)
	service := testService(t, &fakeStore{}, now)
	tests := []struct {
		name    string
		player  string
		key     string
		element string
		code    string
	}{
		{"identity", "", testKey, StarterElementFire, "identity_invalid"},
		{"key", testPlayerID, "bad", StarterElementFire, "idempotency_key_invalid"},
		{"empty element", testPlayerID, testKey, "", "starter_element_invalid"},
		{"unreleased element", testPlayerID, testKey, "air", "starter_element_invalid"},
		{"case changed", testPlayerID, testKey, "Fire", "starter_element_invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := service.SelectStarterEgg(
				context.Background(),
				test.player,
				test.key,
				StarterEggRequest{Element: test.element},
			)
			var apiErr *Error
			if !errors.As(err, &apiErr) || apiErr.Code != test.code {
				t.Fatalf("error = %#v, want code %q", err, test.code)
			}
		})
	}

	store := &fakeStore{starterErr: ErrParentalConsentRequired}
	service = testService(t, store, now)
	_, err := service.SelectStarterEgg(
		context.Background(),
		testPlayerID,
		testKey,
		StarterEggRequest{Element: StarterElementFire},
	)
	var apiErr *Error
	if !errors.As(err, &apiErr) ||
		apiErr.Code != "parental_consent_required" ||
		apiErr.HTTPStatus != 403 {
		t.Fatalf("mapped error = %#v", err)
	}
}

func testService(t *testing.T, store Store, now time.Time) *Service {
	t.Helper()
	service, err := NewService(ServiceConfig{
		Store:  store,
		Core:   fakeCore{},
		Now:    func() time.Time { return now },
		Random: bytes.NewReader(make([]byte, 512)),
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}

type fakeStore struct {
	ageResponse     AgeGateResponse
	ageErr          error
	ageCommit       AgeGateCommit
	starterResponse StarterEggResponse
	starterErr      error
	starterCommit   StarterEggCommit
}

func (s *fakeStore) RecordAgeGate(
	_ context.Context,
	input AgeGateCommit,
) (AgeGateResponse, error) {
	s.ageCommit = input
	return s.ageResponse, s.ageErr
}

func (s *fakeStore) SelectStarterEgg(
	_ context.Context,
	input StarterEggCommit,
	_ corebridge.StarterEngine,
) (StarterEggResponse, error) {
	s.starterCommit = input
	return s.starterResponse, s.starterErr
}

type fakeCore struct{}

func (fakeCore) GenerateStarterGenome(
	context.Context,
	uint8,
	uint64,
) (corebridge.Genome, error) {
	return corebridge.Genome{}, nil
}
