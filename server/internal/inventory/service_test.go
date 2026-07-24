package inventory

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/gochya/gochya/server/internal/dojo"
)

const inventoryTestPlayer = "11111111-1111-4111-8111-111111111111"

func TestListTechniquesUsesStableCursorPage(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	store := &fakeTechniqueStore{
		cards: []dojo.TechniqueCard{
			{ID: "33333333-3333-4333-8333-333333333333", CreatedAt: now},
			{ID: "22222222-2222-4222-8222-222222222222", CreatedAt: now},
			{
				ID:        "11111111-1111-4111-8111-111111111111",
				CreatedAt: now.Add(-time.Minute),
			},
		},
	}
	service, err := NewService(store)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	response, err := service.ListTechniques(
		context.Background(),
		inventoryTestPlayer,
		"2",
		"",
	)
	if err != nil {
		t.Fatalf("ListTechniques: %v", err)
	}
	if len(response.Items) != 2 ||
		response.Items[0].ID != store.cards[0].ID ||
		response.Items[1].ID != store.cards[1].ID ||
		response.NextCursor == "" {
		t.Fatalf("response = %#v", response)
	}
	if store.limit != 3 || store.playerID != inventoryTestPlayer || store.cursor != nil {
		t.Fatalf(
			"store input = player %q, cursor %#v, limit %d",
			store.playerID,
			store.cursor,
			store.limit,
		)
	}
	cursor, err := decodeCursor(response.NextCursor)
	if err != nil {
		t.Fatalf("decodeCursor: %v", err)
	}
	if cursor.ID != response.Items[1].ID ||
		!cursor.CreatedAt.Equal(response.Items[1].CreatedAt) {
		t.Fatalf("cursor = %#v", cursor)
	}
}

func TestListTechniquesPassesDecodedCursor(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	cursorValue, err := encodeCursor(TechniqueCursor{
		CreatedAt: now,
		ID:        "22222222-2222-4222-8222-222222222222",
	})
	if err != nil {
		t.Fatalf("encodeCursor: %v", err)
	}
	store := &fakeTechniqueStore{}
	service, err := NewService(store)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	response, err := service.ListTechniques(
		context.Background(),
		inventoryTestPlayer,
		"",
		cursorValue,
	)
	if err != nil {
		t.Fatalf("ListTechniques: %v", err)
	}
	if len(response.Items) != 0 || response.Items == nil || response.NextCursor != "" {
		t.Fatalf("response = %#v", response)
	}
	if store.limit != defaultPageLimit+1 ||
		store.cursor == nil ||
		store.cursor.ID != "22222222-2222-4222-8222-222222222222" ||
		!store.cursor.CreatedAt.Equal(now) {
		t.Fatalf("store cursor/limit = %#v/%d", store.cursor, store.limit)
	}
}

func TestListTechniquesRejectsInvalidPagination(t *testing.T) {
	service, err := NewService(&fakeTechniqueStore{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	tests := []struct {
		name   string
		limit  string
		cursor string
		code   string
	}{
		{name: "zero limit", limit: "0", code: "invalid_limit"},
		{name: "large limit", limit: "101", code: "invalid_limit"},
		{name: "non-number limit", limit: "many", code: "invalid_limit"},
		{name: "invalid cursor", cursor: "not-a-cursor", code: "invalid_cursor"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := service.ListTechniques(
				context.Background(),
				inventoryTestPlayer,
				test.limit,
				test.cursor,
			)
			var apiErr *Error
			if !errors.As(err, &apiErr) || apiErr.Code != test.code {
				t.Fatalf("error = %v, want %q", err, test.code)
			}
		})
	}
}

func TestCursorRejectsUnknownFieldsAndNonCanonicalEncoding(t *testing.T) {
	valid, err := encodeCursor(TechniqueCursor{
		CreatedAt: time.Now().UTC(),
		ID:        "22222222-2222-4222-8222-222222222222",
	})
	if err != nil {
		t.Fatalf("encodeCursor: %v", err)
	}
	if _, err := decodeCursor(valid + "="); err == nil {
		t.Fatal("padded cursor was accepted")
	}
	unknown := "eyJjcmVhdGVkX2F0IjoiMjAyNi0wNy0yM1QxMjowMDowMFoiLCJpZCI6" +
		"IjIyMjIyMjIyLTIyMjItNDIyMi04MjIyLTIyMjIyMjIyMjIyMiIsImV4dHJhIjp0cnVlfQ"
	if _, err := decodeCursor(unknown); err == nil {
		t.Fatal("cursor with an unknown field was accepted")
	}
}

func TestNewServiceRequiresStore(t *testing.T) {
	if _, err := NewService(nil); err == nil {
		t.Fatal("NewService accepted nil store")
	}
}

func TestEquipTechniquesValidatesAndCallsStore(t *testing.T) {
	cardIDs := testLoadoutCardIDs()
	expected := LoadoutResponse{
		PetID:        "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		CardIDs:      append([]string(nil), cardIDs...),
		SignatureIdx: 2,
		Revision:     1,
		UpdatedAt:    time.Now().UTC(),
	}
	store := &fakeTechniqueStore{equipResponse: expected}
	service, err := NewService(store)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	response, err := service.EquipTechniques(
		context.Background(),
		inventoryTestPlayer,
		"99999999-9999-4999-8999-999999999999",
		EquipTechniquesRequest{CardIDs: cardIDs, SignatureIdx: 2},
	)
	if err != nil {
		t.Fatalf("EquipTechniques: %v", err)
	}
	if !reflect.DeepEqual(response, expected) {
		t.Fatalf("response = %#v", response)
	}
	if store.equipInput.PlayerID != inventoryTestPlayer ||
		store.equipInput.SignatureIdx != 2 ||
		store.equipInput.RequestHash == ([32]byte{}) ||
		!reflect.DeepEqual(store.equipInput.CardIDs, cardIDs) {
		t.Fatalf("equip input = %#v", store.equipInput)
	}
	cardIDs[0] = "ffffffff-ffff-4fff-8fff-ffffffffffff"
	if store.equipInput.CardIDs[0] == cardIDs[0] {
		t.Fatal("service did not isolate card IDs from caller mutation")
	}
}

func TestEquipTechniquesRejectsInvalidEnvelope(t *testing.T) {
	validCards := testLoadoutCardIDs()
	tests := []struct {
		name    string
		key     string
		request EquipTechniquesRequest
		code    string
	}{
		{
			name:    "invalid idempotency key",
			key:     "not-a-uuid",
			request: EquipTechniquesRequest{CardIDs: validCards, SignatureIdx: 2},
			code:    "idempotency_key_invalid",
		},
		{
			name: "four cards",
			key:  "99999999-9999-4999-8999-999999999999",
			request: EquipTechniquesRequest{
				CardIDs:      validCards[:4],
				SignatureIdx: 2,
			},
			code: "loadout_invalid",
		},
		{
			name: "duplicate card",
			key:  "99999999-9999-4999-8999-999999999999",
			request: EquipTechniquesRequest{
				CardIDs: []string{
					validCards[0],
					validCards[0],
					validCards[2],
					validCards[3],
					validCards[4],
				},
				SignatureIdx: 2,
			},
			code: "loadout_invalid",
		},
		{
			name: "invalid card UUID",
			key:  "99999999-9999-4999-8999-999999999999",
			request: EquipTechniquesRequest{
				CardIDs: []string{
					"bad",
					validCards[1],
					validCards[2],
					validCards[3],
					validCards[4],
				},
				SignatureIdx: 2,
			},
			code: "loadout_invalid",
		},
		{
			name: "signature out of range",
			key:  "99999999-9999-4999-8999-999999999999",
			request: EquipTechniquesRequest{
				CardIDs:      validCards,
				SignatureIdx: 5,
			},
			code: "loadout_invalid",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &fakeTechniqueStore{}
			service, err := NewService(store)
			if err != nil {
				t.Fatalf("NewService: %v", err)
			}
			_, err = service.EquipTechniques(
				context.Background(),
				inventoryTestPlayer,
				test.key,
				test.request,
			)
			var apiErr *Error
			if !errors.As(err, &apiErr) || apiErr.Code != test.code {
				t.Fatalf("error = %v, want %q", err, test.code)
			}
			if store.equipCalls != 0 {
				t.Fatalf("store equip calls = %d", store.equipCalls)
			}
		})
	}
}

func TestCurrentLoadoutMapsNotFound(t *testing.T) {
	store := &fakeTechniqueStore{currentErr: ErrLoadoutNotFound}
	service, err := NewService(store)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	_, err = service.CurrentLoadout(context.Background(), inventoryTestPlayer)
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "loadout_not_found" {
		t.Fatalf("error = %v", err)
	}
}

type fakeTechniqueStore struct {
	cards           []dojo.TechniqueCard
	err             error
	playerID        string
	cursor          *TechniqueCursor
	limit           int
	equipResponse   LoadoutResponse
	equipErr        error
	equipInput      EquipCommit
	equipCalls      int
	currentResponse LoadoutResponse
	currentErr      error
}

func (s *fakeTechniqueStore) ListTechniqueCards(
	_ context.Context,
	playerID string,
	cursor *TechniqueCursor,
	limit int,
) ([]dojo.TechniqueCard, error) {
	s.playerID = playerID
	s.cursor = cursor
	s.limit = limit
	if s.err != nil {
		return nil, s.err
	}
	result := make([]dojo.TechniqueCard, len(s.cards))
	copy(result, s.cards)
	return result, nil
}

func (s *fakeTechniqueStore) EquipTechniques(
	_ context.Context,
	input EquipCommit,
) (LoadoutResponse, error) {
	s.equipCalls++
	s.equipInput = input
	s.equipInput.CardIDs = append([]string(nil), input.CardIDs...)
	return s.equipResponse, s.equipErr
}

func (s *fakeTechniqueStore) CurrentLoadout(
	_ context.Context,
	_ string,
) (LoadoutResponse, error) {
	return s.currentResponse, s.currentErr
}

func TestServicePreservesStoreErrorsAsInternal(t *testing.T) {
	storeErr := errors.New("database unavailable")
	service, err := NewService(&fakeTechniqueStore{err: storeErr})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	_, err = service.ListTechniques(context.Background(), inventoryTestPlayer, "", "")
	var apiErr *Error
	if !errors.As(err, &apiErr) ||
		apiErr.Code != "internal_error" ||
		!errors.Is(err, storeErr) {
		t.Fatalf("error = %#v", err)
	}
}

func testLoadoutCardIDs() []string {
	return []string{
		"11111111-1111-4111-8111-111111111111",
		"22222222-2222-4222-8222-222222222222",
		"33333333-3333-4333-8333-333333333333",
		"44444444-4444-4444-8444-444444444444",
		"55555555-5555-4555-8555-555555555555",
	}
}
