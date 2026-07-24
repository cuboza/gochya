package profile

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

const (
	testPlayerID = "11111111-1111-4111-8111-111111111111"
	testPetID    = "22222222-2222-4222-8222-222222222222"
)

func TestServiceReadsProfileAndPets(t *testing.T) {
	profile := PlayerProfile{ID: testPlayerID, Username: "player"}
	pet := testPet(testPetID, true)
	lineage := testLineage()
	store := &fakeStore{
		profile: profile,
		pets:    []Pet{pet},
		pet:     pet,
		lineage: lineage,
	}
	service := testService(t, store)

	gotProfile, err := service.PlayerProfile(context.Background(), testPlayerID)
	if err != nil {
		t.Fatalf("PlayerProfile: %v", err)
	}
	if !reflect.DeepEqual(gotProfile, profile) {
		t.Fatalf("profile = %#v", gotProfile)
	}
	pets, err := service.ListPets(context.Background(), testPlayerID)
	if err != nil {
		t.Fatalf("ListPets: %v", err)
	}
	if !reflect.DeepEqual(pets, []Pet{pet}) {
		t.Fatalf("pets = %#v", pets)
	}
	gotPet, err := service.Pet(context.Background(), testPlayerID, testPetID)
	if err != nil {
		t.Fatalf("Pet: %v", err)
	}
	if !reflect.DeepEqual(gotPet, pet) {
		t.Fatalf("pet = %#v", gotPet)
	}
	gotLineage, err := service.Lineage(
		context.Background(),
		testPlayerID,
		testPetID,
	)
	if err != nil {
		t.Fatalf("Lineage: %v", err)
	}
	if !reflect.DeepEqual(gotLineage, lineage) {
		t.Fatalf("lineage = %#v", gotLineage)
	}
	if store.playerID != testPlayerID || store.petID != testPetID {
		t.Fatalf("store IDs = %q/%q", store.playerID, store.petID)
	}
}

func TestServiceListPetsReturnsJSONArrayForEmptyInventory(t *testing.T) {
	service := testService(t, &fakeStore{})
	pets, err := service.ListPets(context.Background(), testPlayerID)
	if err != nil {
		t.Fatalf("ListPets: %v", err)
	}
	if pets == nil || len(pets) != 0 {
		t.Fatalf("pets = %#v", pets)
	}
}

func TestServiceActivatesPetWithAuthoritativeTime(t *testing.T) {
	now := time.Date(2026, time.July, 24, 9, 10, 11, 12, time.FixedZone("test", 4*60*60))
	expected := testPet(testPetID, true)
	store := &fakeStore{activated: expected}
	service, err := NewService(ServiceConfig{
		Store: store,
		Now:   func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	got, err := service.ActivatePet(context.Background(), testPlayerID, testPetID)
	if err != nil {
		t.Fatalf("ActivatePet: %v", err)
	}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("pet = %#v", got)
	}
	if store.activate.PlayerID != testPlayerID ||
		store.activate.PetID != testPetID ||
		store.activate.Now.Location() != time.UTC ||
		!store.activate.Now.Equal(now) {
		t.Fatalf("activate input = %#v", store.activate)
	}
}

func TestServiceValidatesIdentityAndPetID(t *testing.T) {
	store := &fakeStore{}
	service := testService(t, store)
	tests := []struct {
		name      string
		operation func() error
		code      string
	}{
		{
			name: "missing identity",
			operation: func() error {
				_, err := service.PlayerProfile(context.Background(), "")
				return err
			},
			code: "identity_invalid",
		},
		{
			name: "invalid pet ID on get",
			operation: func() error {
				_, err := service.Pet(context.Background(), testPlayerID, "bad")
				return err
			},
			code: "pet_id_invalid",
		},
		{
			name: "invalid pet ID on activate",
			operation: func() error {
				_, err := service.ActivatePet(
					context.Background(),
					testPlayerID,
					"bad",
				)
				return err
			},
			code: "pet_id_invalid",
		},
		{
			name: "invalid pet ID on lineage",
			operation: func() error {
				_, err := service.Lineage(
					context.Background(),
					testPlayerID,
					"bad",
				)
				return err
			},
			code: "pet_id_invalid",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertErrorCode(t, test.operation(), test.code)
		})
	}
	if store.calls != 0 {
		t.Fatalf("store calls = %d", store.calls)
	}
}

func TestServiceMapsNotFoundAndInternalErrors(t *testing.T) {
	databaseErr := errors.New("database unavailable")
	tests := []struct {
		name  string
		store *fakeStore
		call  func(*Service) error
		code  string
		cause error
	}{
		{
			name:  "profile missing",
			store: &fakeStore{profileErr: ErrPlayerNotFound},
			call: func(service *Service) error {
				_, err := service.PlayerProfile(context.Background(), testPlayerID)
				return err
			},
			code: "profile_not_found",
		},
		{
			name:  "pet missing",
			store: &fakeStore{petErr: ErrPetNotFound},
			call: func(service *Service) error {
				_, err := service.Pet(context.Background(), testPlayerID, testPetID)
				return err
			},
			code: "pet_not_found",
		},
		{
			name:  "lineage root missing",
			store: &fakeStore{lineageErr: ErrPetNotFound},
			call: func(service *Service) error {
				_, err := service.Lineage(
					context.Background(),
					testPlayerID,
					testPetID,
				)
				return err
			},
			code: "pet_not_found",
		},
		{
			name:  "lineage invalid",
			store: &fakeStore{lineageErr: ErrLineageInvalid},
			call: func(service *Service) error {
				_, err := service.Lineage(
					context.Background(),
					testPlayerID,
					testPetID,
				)
				return err
			},
			code:  "internal_error",
			cause: ErrLineageInvalid,
		},
		{
			name:  "activation profile missing",
			store: &fakeStore{activateErr: ErrPlayerNotFound},
			call: func(service *Service) error {
				_, err := service.ActivatePet(
					context.Background(),
					testPlayerID,
					testPetID,
				)
				return err
			},
			code: "profile_not_found",
		},
		{
			name:  "database failure",
			store: &fakeStore{listErr: databaseErr},
			call: func(service *Service) error {
				_, err := service.ListPets(context.Background(), testPlayerID)
				return err
			},
			code:  "internal_error",
			cause: databaseErr,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.call(testService(t, test.store))
			assertErrorCode(t, err, test.code)
			if test.cause != nil && !errors.Is(err, test.cause) {
				t.Fatalf("error %v does not wrap %v", err, test.cause)
			}
		})
	}
}

func TestNewServiceRequiresStore(t *testing.T) {
	if _, err := NewService(ServiceConfig{}); err == nil {
		t.Fatal("NewService accepted nil store")
	}
}

type fakeStore struct {
	profile     PlayerProfile
	profileErr  error
	pets        []Pet
	listErr     error
	pet         Pet
	petErr      error
	lineage     LineageTree
	lineageErr  error
	activated   Pet
	activateErr error
	playerID    string
	petID       string
	activate    ActivatePetCommit
	calls       int
}

func (s *fakeStore) PlayerProfile(
	_ context.Context,
	playerID string,
) (PlayerProfile, error) {
	s.calls++
	s.playerID = playerID
	return s.profile, s.profileErr
}

func (s *fakeStore) ListPets(_ context.Context, playerID string) ([]Pet, error) {
	s.calls++
	s.playerID = playerID
	return s.pets, s.listErr
}

func (s *fakeStore) Pet(
	_ context.Context,
	playerID string,
	petID string,
) (Pet, error) {
	s.calls++
	s.playerID = playerID
	s.petID = petID
	return s.pet, s.petErr
}

func (s *fakeStore) Lineage(
	_ context.Context,
	playerID string,
	petID string,
) (LineageTree, error) {
	s.calls++
	s.playerID = playerID
	s.petID = petID
	return s.lineage, s.lineageErr
}

func (s *fakeStore) ActivatePet(
	_ context.Context,
	input ActivatePetCommit,
) (Pet, error) {
	s.calls++
	s.activate = input
	return s.activated, s.activateErr
}

func testService(t *testing.T, store Store) *Service {
	t.Helper()
	service, err := NewService(ServiceConfig{Store: store})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}

func testLineage() LineageTree {
	return LineageTree{
		RootID:   testPetID,
		MaxDepth: lineageMaxDepth,
		Nodes: []LineageNode{{
			ID:            testPetID,
			Genome:        []byte(`{"element":"Earth"}`),
			Stage:         "baby",
			Level:         1,
			AncestorDepth: 0,
		}},
	}
}

func testPet(id string, active bool) Pet {
	return Pet{
		ID:         id,
		OwnerID:    testPlayerID,
		Genome:     []byte(`{"element":"Earth"}`),
		Stage:      "baby",
		Level:      1,
		Needs:      Needs{Hunger: 80, Energy: 70, Hygiene: 60, Mood: 90},
		Stats:      Stats{Strength: 1, Agility: 2, Endurance: 3, Focus: 4},
		IsActive:   active,
		CreatedAt:  time.Date(2026, time.July, 24, 5, 0, 0, 0, time.UTC),
		Generation: 0,
	}
}

func assertErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != code {
		t.Fatalf("error = %v, want code %q", err, code)
	}
}
