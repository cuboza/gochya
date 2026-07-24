package breeding

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gochya/gochya/server/internal/dojo"
)

func TestHTTPBreedingFlow(t *testing.T) {
	now := time.Date(2026, time.July, 24, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{
		breedResponse: BreedResponse{
			EggID:         testEggID,
			IncubateUntil: now.Add(4 * time.Hour),
		},
		eggs: []Egg{{ID: testEggID}},
		pet:  Pet{ID: testParentA, Stage: "baby"},
	}
	routes := breedingRoutes(t, store, now)
	breed := authenticatedRequest(
		routes,
		http.MethodPost,
		"/v1/breeding/breed",
		[]byte(`{"parentA":"`+testParentA+`","parentB":"`+testParentB+`","catalysts":[]}`),
		testKey,
	)
	if breed.Code != http.StatusOK {
		t.Fatalf("breed status = %d, body = %s", breed.Code, breed.Body)
	}
	var breedResponse BreedResponse
	if err := json.Unmarshal(breed.Body.Bytes(), &breedResponse); err != nil {
		t.Fatalf("decode breed response: %v", err)
	}
	if breedResponse.EggID != testEggID {
		t.Fatalf("breed response = %#v", breedResponse)
	}
	eggs := authenticatedRequest(
		routes,
		http.MethodGet,
		"/v1/me/eggs",
		nil,
		"",
	)
	if eggs.Code != http.StatusOK {
		t.Fatalf("eggs status = %d, body = %s", eggs.Code, eggs.Body)
	}
	hatch := authenticatedRequest(
		routes,
		http.MethodPost,
		"/v1/me/eggs/"+testEggID+"/hatch",
		nil,
		"",
	)
	if hatch.Code != http.StatusOK {
		t.Fatalf("hatch status = %d, body = %s", hatch.Code, hatch.Body)
	}
	for name, response := range map[string]*httptest.ResponseRecorder{
		"breed": breed,
		"eggs":  eggs,
		"hatch": hatch,
	} {
		if response.Header().Get("Cache-Control") != "private, no-store" ||
			response.Header().Get("X-Request-ID") == "" {
			t.Fatalf("%s headers = %#v", name, response.Header())
		}
	}
}

func TestHTTPBreedingBoundaries(t *testing.T) {
	routes := breedingRoutes(t, &fakeStore{}, time.Now())
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
		{
			name:       "auth required",
			method:     http.MethodGet,
			target:     "/v1/me/eggs",
			wantStatus: http.StatusUnauthorized,
			wantCode:   "unauthenticated",
		},
		{
			name:       "strict body",
			method:     http.MethodPost,
			target:     "/v1/breeding/breed",
			body:       []byte(`{"parentA":"x","parentB":"y","extra":true}`),
			auth:       true,
			key:        testKey,
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_body",
		},
		{
			name:       "breed method",
			method:     http.MethodGet,
			target:     "/v1/breeding/breed",
			auth:       true,
			wantStatus: http.StatusMethodNotAllowed,
			wantCode:   "method_not_allowed",
		},
		{
			name:       "list query",
			method:     http.MethodGet,
			target:     "/v1/me/eggs?limit=1",
			auth:       true,
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_query",
		},
		{
			name:       "hatch body",
			method:     http.MethodPost,
			target:     "/v1/me/eggs/" + testEggID + "/hatch",
			body:       []byte(`{}`),
			auth:       true,
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_body",
		},
		{
			name:       "unknown egg route",
			method:     http.MethodPost,
			target:     "/v1/me/eggs/" + testEggID + "/skip",
			auth:       true,
			wantStatus: http.StatusNotFound,
			wantCode:   "route_not_found",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(
				test.method,
				test.target,
				bytes.NewReader(test.body),
			)
			if test.auth {
				request.Header.Set("X-Player-ID", testPlayerID)
			}
			if test.key != "" {
				request.Header.Set("Idempotency-Key", test.key)
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
				t.Fatalf("decode error response: %v", err)
			}
			if envelope.Error.Code != test.wantCode {
				t.Fatalf("code = %q, want %q", envelope.Error.Code, test.wantCode)
			}
		})
	}
}

func breedingRoutes(
	t *testing.T,
	store Store,
	now time.Time,
) http.Handler {
	t.Helper()
	service, err := NewService(ServiceConfig{
		Store:  store,
		Core:   fakeCore{},
		Now:    func() time.Time { return now },
		Random: bytes.NewReader(make([]byte, 256)),
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	handler, err := NewHTTPHandler(
		service,
		dojo.HeaderAuthenticator{},
		bytes.NewReader(make([]byte, 256)),
	)
	if err != nil {
		t.Fatalf("NewHTTPHandler: %v", err)
	}
	return handler.Routes()
}

func authenticatedRequest(
	handler http.Handler,
	method string,
	target string,
	body []byte,
	key string,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	request.Header.Set("X-Player-ID", testPlayerID)
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
