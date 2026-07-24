package shop

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gochya/gochya/server/internal/dojo"
)

func TestHTTPShopEndpoints(t *testing.T) {
	now := time.Date(2026, time.July, 24, 12, 30, 0, 0, time.UTC)
	store := &fakeStore{
		purchase: PurchaseResponse{
			ItemID:            ItemApple,
			PurchasedQuantity: 2,
			ItemQuantity:      2,
			UnitPriceKoins:    20,
			KoinsSpent:        40,
			KoinsRemaining:    60,
			PurchasedAt:       now,
		},
		inventory: InventoryResponse{
			Koins: 60,
			Items: []OwnedItem{{ItemID: ItemApple, Quantity: 2}},
		},
	}
	handler := shopRoutes(t, store, now)

	for _, target := range []string{"/v1/shop", "/v1/me/items"} {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		request.Header.Set("X-Player-ID", testPlayerID)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s status = %d, body = %s", target, response.Code, response.Body)
		}
	}

	body, _ := json.Marshal(PurchaseRequest{ItemID: ItemApple, Quantity: 2})
	request := httptest.NewRequest(http.MethodPost, "/v1/shop/buy", bytes.NewReader(body))
	request.Header.Set("X-Player-ID", testPlayerID)
	request.Header.Set("Idempotency-Key", testKey)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("purchase status = %d, body = %s", response.Code, response.Body)
	}
	var purchase PurchaseResponse
	if err := json.Unmarshal(response.Body.Bytes(), &purchase); err != nil ||
		purchase.KoinsSpent != 40 ||
		purchase.ItemQuantity != 2 {
		t.Fatalf("purchase = %#v, %v", purchase, err)
	}
	if response.Header().Get("Cache-Control") != "private, no-store" ||
		response.Header().Get("X-Request-ID") == "" {
		t.Fatalf("headers = %#v", response.Header())
	}
}

func TestHTTPShopBoundaries(t *testing.T) {
	validBody := []byte(`{"itemId":"apple","quantity":1}`)
	tests := []struct {
		name       string
		method     string
		target     string
		body       []byte
		auth       bool
		key        string
		wantStatus int
		wantCode   string
	}{
		{"auth", http.MethodGet, "/v1/shop", nil, false, "", 401, "unauthenticated"},
		{"catalog method", http.MethodPost, "/v1/shop", nil, true, "", 405, "method_not_allowed"},
		{"catalog query", http.MethodGet, "/v1/shop?x=1", nil, true, "", 400, "invalid_query"},
		{"purchase method", http.MethodGet, "/v1/shop/buy", nil, true, testKey, 405, "method_not_allowed"},
		{"purchase query", http.MethodPost, "/v1/shop/buy?x=1", validBody, true, testKey, 400, "invalid_query"},
		{"purchase body", http.MethodPost, "/v1/shop/buy", []byte(`{"itemId":"apple","quantity":1,"extra":1}`), true, testKey, 400, "invalid_body"},
		{"purchase key", http.MethodPost, "/v1/shop/buy", validBody, true, "", 400, "idempotency_key_invalid"},
		{"inventory method", http.MethodPost, "/v1/me/items", nil, true, "", 405, "method_not_allowed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.target, bytes.NewReader(test.body))
			if test.auth {
				request.Header.Set("X-Player-ID", testPlayerID)
			}
			request.Header.Set("Idempotency-Key", test.key)
			response := httptest.NewRecorder()
			shopRoutes(t, &fakeStore{}, time.Time{}).ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body)
			}
			var envelope struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil ||
				envelope.Error.Code != test.wantCode {
				t.Fatalf("error = %#v, %v", envelope, err)
			}
		})
	}
}

func shopRoutes(t *testing.T, store Store, now time.Time) http.Handler {
	t.Helper()
	handler, err := NewHTTPHandler(
		shopService(t, store, now),
		dojo.HeaderAuthenticator{},
		bytes.NewReader(make([]byte, 512)),
	)
	if err != nil {
		t.Fatalf("NewHTTPHandler: %v", err)
	}
	return handler.Routes()
}
