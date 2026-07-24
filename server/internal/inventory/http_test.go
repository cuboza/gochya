package inventory

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gochya/gochya/server/internal/dojo"
)

func TestHTTPListsAuthenticatedTechniqueInventory(t *testing.T) {
	store := &fakeTechniqueStore{cards: []dojo.TechniqueCard{{
		ID:        "22222222-2222-4222-8222-222222222222",
		OwnerID:   inventoryTestPlayer,
		Type:      1,
		Quality:   80,
		CreatedAt: time.Now().UTC(),
	}}}
	service, err := NewService(store)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	handler, err := NewHTTPHandler(service, dojo.HeaderAuthenticator{}, nil)
	if err != nil {
		t.Fatalf("NewHTTPHandler: %v", err)
	}
	request := httptest.NewRequest(
		http.MethodGet,
		"/v1/me/techniques?limit=20",
		nil,
	)
	request.Header.Set("X-Player-ID", inventoryTestPlayer)
	response := httptest.NewRecorder()

	handler.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body)
	}
	var page ListTechniquesResponse
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != store.cards[0].ID {
		t.Fatalf("page = %#v", page)
	}
	if response.Header().Get("Cache-Control") != "private, no-store" ||
		response.Header().Get("X-Request-ID") == "" {
		t.Fatalf("headers = %#v", response.Header())
	}
}

func TestHTTPTechniqueInventoryBoundaries(t *testing.T) {
	service, err := NewService(&fakeTechniqueStore{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	handler, err := NewHTTPHandler(service, dojo.HeaderAuthenticator{}, nil)
	if err != nil {
		t.Fatalf("NewHTTPHandler: %v", err)
	}
	routes := handler.Routes()
	tests := []struct {
		name       string
		method     string
		target     string
		playerID   string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "authentication required",
			method:     http.MethodGet,
			target:     "/v1/me/techniques",
			wantStatus: http.StatusUnauthorized,
			wantCode:   "unauthenticated",
		},
		{
			name:       "method rejected",
			method:     http.MethodPost,
			target:     "/v1/me/techniques",
			playerID:   inventoryTestPlayer,
			wantStatus: http.StatusMethodNotAllowed,
			wantCode:   "method_not_allowed",
		},
		{
			name:       "unknown query rejected",
			method:     http.MethodGet,
			target:     "/v1/me/techniques?offset=20",
			playerID:   inventoryTestPlayer,
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_query",
		},
		{
			name:       "duplicate query rejected",
			method:     http.MethodGet,
			target:     "/v1/me/techniques?limit=10&limit=20",
			playerID:   inventoryTestPlayer,
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_query",
		},
		{
			name:       "invalid cursor rejected",
			method:     http.MethodGet,
			target:     "/v1/me/techniques?cursor=broken",
			playerID:   inventoryTestPlayer,
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_cursor",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.target, nil)
			if test.playerID != "" {
				request.Header.Set("X-Player-ID", test.playerID)
			}
			response := httptest.NewRecorder()
			routes.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body)
			}
			var envelope struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("decode error: %v", err)
			}
			if !strings.EqualFold(envelope.Error.Code, test.wantCode) {
				t.Fatalf("error code = %q", envelope.Error.Code)
			}
		})
	}
}

func TestHTTPEquipsAndReadsCurrentLoadout(t *testing.T) {
	loadout := LoadoutResponse{
		PetID:        "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		CardIDs:      testLoadoutCardIDs(),
		SignatureIdx: 2,
		Revision:     1,
		UpdatedAt:    time.Now().UTC(),
	}
	store := &fakeTechniqueStore{
		equipResponse:   loadout,
		currentResponse: loadout,
	}
	service, err := NewService(store)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	handler, err := NewHTTPHandler(service, dojo.HeaderAuthenticator{}, nil)
	if err != nil {
		t.Fatalf("NewHTTPHandler: %v", err)
	}
	routes := handler.Routes()
	input, err := json.Marshal(EquipTechniquesRequest{
		CardIDs:      loadout.CardIDs,
		SignatureIdx: loadout.SignatureIdx,
	})
	if err != nil {
		t.Fatalf("marshal equip: %v", err)
	}
	equipRequest := httptest.NewRequest(
		http.MethodPost,
		"/v1/me/techniques/equip",
		bytes.NewReader(input),
	)
	equipRequest.Header.Set("X-Player-ID", inventoryTestPlayer)
	equipRequest.Header.Set(
		"Idempotency-Key",
		"99999999-9999-4999-8999-999999999999",
	)
	equipResponse := httptest.NewRecorder()
	routes.ServeHTTP(equipResponse, equipRequest)
	if equipResponse.Code != http.StatusOK {
		t.Fatalf("equip status = %d, body = %s", equipResponse.Code, equipResponse.Body)
	}
	var equipped LoadoutResponse
	if err := json.Unmarshal(equipResponse.Body.Bytes(), &equipped); err != nil {
		t.Fatalf("decode equip: %v", err)
	}
	if equipped.Revision != 1 || equipped.SignatureIdx != 2 {
		t.Fatalf("equipped = %#v", equipped)
	}

	currentRequest := httptest.NewRequest(http.MethodGet, "/v1/me/loadout", nil)
	currentRequest.Header.Set("X-Player-ID", inventoryTestPlayer)
	currentResponse := httptest.NewRecorder()
	routes.ServeHTTP(currentResponse, currentRequest)
	if currentResponse.Code != http.StatusOK {
		t.Fatalf(
			"current status = %d, body = %s",
			currentResponse.Code,
			currentResponse.Body,
		)
	}
	var current LoadoutResponse
	if err := json.Unmarshal(currentResponse.Body.Bytes(), &current); err != nil {
		t.Fatalf("decode current: %v", err)
	}
	if current.Revision != loadout.Revision ||
		current.PetID != loadout.PetID ||
		len(current.CardIDs) != 5 {
		t.Fatalf("current = %#v", current)
	}
}
