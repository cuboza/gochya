package breeding

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gochya/gochya/server/internal/corebridge"
)

const (
	testPlayerID = "11111111-1111-4111-8111-111111111111"
	testParentA  = "22222222-2222-4222-8222-222222222222"
	testParentB  = "33333333-3333-4333-8333-333333333333"
	testEggID    = "44444444-4444-4444-8444-444444444444"
	testKey      = "55555555-5555-4555-8555-555555555555"
)

func TestServiceBreedsListsAndHatches(t *testing.T) {
	now := time.Date(2026, time.July, 24, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{
		breedResponse: BreedResponse{
			EggID:         testEggID,
			IncubateUntil: now.Add(4 * time.Hour),
		},
		eggs: []Egg{{ID: testEggID}},
		pet:  Pet{ID: testParentA, Stage: "baby"},
	}
	service, err := NewService(ServiceConfig{
		Store:  store,
		Core:   fakeCore{},
		Now:    func() time.Time { return now },
		Random: bytes.NewReader(make([]byte, 64)),
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	response, err := service.Breed(
		context.Background(),
		testPlayerID,
		testKey,
		BreedRequest{
			ParentAID: testParentA,
			ParentBID: testParentB,
			Catalysts: []string{"hybrid", "mutation"},
		},
	)
	if err != nil {
		t.Fatalf("Breed: %v", err)
	}
	if !reflect.DeepEqual(response, store.breedResponse) {
		t.Fatalf("response = %#v", response)
	}
	if store.breedCommit.PlayerID != testPlayerID ||
		store.breedCommit.ParentAID != testParentA ||
		store.breedCommit.ParentBID != testParentB ||
		!store.breedCommit.MutationCatalyst ||
		!store.breedCommit.HybridCatalyst ||
		store.breedCommit.IdempotencyKey != testKey ||
		store.breedCommit.EggID == "" ||
		store.breedCommit.Now != now {
		t.Fatalf("commit = %#v", store.breedCommit)
	}
	if store.breedCommit.RequestHash == ([32]byte{}) {
		t.Fatal("request hash is empty")
	}
	if _, err := service.Breed(
		context.Background(),
		testPlayerID,
		strings.ToUpper(testKey),
		BreedRequest{
			ParentAID: strings.ToUpper(testParentA),
			ParentBID: strings.ToUpper(testParentB),
		},
	); err != nil {
		t.Fatalf("Breed uppercase UUIDs: %v", err)
	}
	if store.breedCommit.ParentAID != testParentA ||
		store.breedCommit.ParentBID != testParentB ||
		store.breedCommit.IdempotencyKey != testKey {
		t.Fatalf("normalized commit = %#v", store.breedCommit)
	}
	eggs, err := service.ListEggs(context.Background(), testPlayerID)
	if err != nil || !reflect.DeepEqual(eggs, store.eggs) {
		t.Fatalf("ListEggs = %#v, %v", eggs, err)
	}
	pet, err := service.Hatch(context.Background(), testPlayerID, testEggID)
	if err != nil || !reflect.DeepEqual(pet, store.pet) {
		t.Fatalf("Hatch = %#v, %v", pet, err)
	}
	if store.hatchCommit.EggID != testEggID ||
		store.hatchCommit.PlayerID != testPlayerID ||
		store.hatchCommit.PetID == "" ||
		store.hatchCommit.Now != now {
		t.Fatalf("hatch commit = %#v", store.hatchCommit)
	}
}

func TestServiceRejectsInvalidBreedingRequests(t *testing.T) {
	service, err := NewService(ServiceConfig{
		Store:  &fakeStore{},
		Core:   fakeCore{},
		Random: bytes.NewReader(make([]byte, 256)),
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	tests := []struct {
		name    string
		player  string
		key     string
		request BreedRequest
		code    string
	}{
		{
			name: "identity",
			key:  testKey,
			request: BreedRequest{
				ParentAID: testParentA,
				ParentBID: testParentB,
			},
			code: "identity_invalid",
		},
		{
			name:   "idempotency key",
			player: testPlayerID,
			key:    "bad",
			request: BreedRequest{
				ParentAID: testParentA,
				ParentBID: testParentB,
			},
			code: "idempotency_key_invalid",
		},
		{
			name:   "parent ID",
			player: testPlayerID,
			key:    testKey,
			request: BreedRequest{
				ParentAID: "bad",
				ParentBID: testParentB,
			},
			code: "parent_id_invalid",
		},
		{
			name:   "same parent",
			player: testPlayerID,
			key:    testKey,
			request: BreedRequest{
				ParentAID: testParentA,
				ParentBID: testParentA,
			},
			code: "parents_identical",
		},
		{
			name:   "unknown catalyst",
			player: testPlayerID,
			key:    testKey,
			request: BreedRequest{
				ParentAID: testParentA,
				ParentBID: testParentB,
				Catalysts: []string{"reroll"},
			},
			code: "catalysts_invalid",
		},
		{
			name:   "duplicate catalyst",
			player: testPlayerID,
			key:    testKey,
			request: BreedRequest{
				ParentAID: testParentA,
				ParentBID: testParentB,
				Catalysts: []string{"mutation", "mutation"},
			},
			code: "catalysts_invalid",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := service.Breed(
				context.Background(),
				test.player,
				test.key,
				test.request,
			)
			var apiErr *Error
			if !errors.As(err, &apiErr) || apiErr.Code != test.code {
				t.Fatalf("error = %#v, want code %q", err, test.code)
			}
		})
	}
}

func TestServiceMapsStoreErrors(t *testing.T) {
	store := &fakeStore{breedErr: ErrInsufficientKoins}
	service, err := NewService(ServiceConfig{
		Store:  store,
		Core:   fakeCore{},
		Random: bytes.NewReader(make([]byte, 64)),
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	_, err = service.Breed(
		context.Background(),
		testPlayerID,
		testKey,
		BreedRequest{ParentAID: testParentA, ParentBID: testParentB},
	)
	var apiErr *Error
	if !errors.As(err, &apiErr) ||
		apiErr.Code != "insufficient_koins" ||
		apiErr.HTTPStatus != 409 {
		t.Fatalf("error = %#v", err)
	}
}

type fakeStore struct {
	breedResponse BreedResponse
	breedErr      error
	breedCommit   BreedCommit
	eggs          []Egg
	listErr       error
	pet           Pet
	hatchErr      error
	hatchCommit   HatchCommit
}

func (s *fakeStore) Breed(
	_ context.Context,
	input BreedCommit,
	_ corebridge.BreedingEngine,
) (BreedResponse, error) {
	s.breedCommit = input
	return s.breedResponse, s.breedErr
}

func (s *fakeStore) ListEggs(
	_ context.Context,
	_ string,
) ([]Egg, error) {
	return s.eggs, s.listErr
}

func (s *fakeStore) Hatch(
	_ context.Context,
	input HatchCommit,
) (Pet, error) {
	s.hatchCommit = input
	return s.pet, s.hatchErr
}

type fakeCore struct{}

func (fakeCore) Breed(
	context.Context,
	corebridge.BreedInput,
	uint64,
) (corebridge.BreedResult, error) {
	return corebridge.BreedResult{}, nil
}
