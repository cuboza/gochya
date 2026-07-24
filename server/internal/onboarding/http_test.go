package onboarding

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gochya/gochya/server/internal/dojo"
)

func TestHTTPOnboardingFlow(t *testing.T) {
	now := time.Date(2026, time.July, 24, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{
		ageResponse: AgeGateResponse{
			Status:     AgeStatusEligible,
			RecordedAt: now,
		},
		starterResponse: StarterEggResponse{
			EggID:         testEggID,
			Element:       StarterElementEarth,
			IncubateUntil: now.Add(starterIncubation),
		},
	}
	routes := onboardingRoutes(t, store, now)
	age := onboardingRequest(
		routes,
		http.MethodPost,
		"/v1/me/onboarding/age-gate",
		[]byte(`{"birthDate":"2000-01-01"}`),
		true,
		testKey,
	)
	if age.Code != http.StatusOK {
		t.Fatalf("age status = %d, body = %s", age.Code, age.Body)
	}
	starter := onboardingRequest(
		routes,
		http.MethodPost,
		"/v1/me/onboarding/starter-egg",
		[]byte(`{"element":"earth"}`),
		true,
		testKey,
	)
	if starter.Code != http.StatusOK {
		t.Fatalf("starter status = %d, body = %s", starter.Code, starter.Body)
	}
	var response StarterEggResponse
	if err := json.Unmarshal(starter.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode starter response: %v", err)
	}
	if response != store.starterResponse {
		t.Fatalf("starter response = %#v", response)
	}
	for name, recorder := range map[string]*httptest.ResponseRecorder{
		"age":     age,
		"starter": starter,
	} {
		if recorder.Header().Get("Cache-Control") != "private, no-store" ||
			recorder.Header().Get("X-Request-ID") == "" {
			t.Fatalf("%s headers = %#v", name, recorder.Header())
		}
	}
}

func TestHTTPOnboardingBoundaries(t *testing.T) {
	routes := onboardingRoutes(t, &fakeStore{}, time.Now())
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
			method:     http.MethodPost,
			target:     "/v1/me/onboarding/age-gate",
			body:       []byte(`{"birthDate":"2000-01-01"}`),
			key:        testKey,
			wantStatus: http.StatusUnauthorized,
			wantCode:   "unauthenticated",
		},
		{
			name:       "age strict body",
			method:     http.MethodPost,
			target:     "/v1/me/onboarding/age-gate",
			body:       []byte(`{"birthDate":"2000-01-01","extra":true}`),
			auth:       true,
			key:        testKey,
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_body",
		},
		{
			name:       "starter strict body",
			method:     http.MethodPost,
			target:     "/v1/me/onboarding/starter-egg",
			body:       []byte(`{"element":"fire"} {}`),
			auth:       true,
			key:        testKey,
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_body",
		},
		{
			name:       "missing idempotency key",
			method:     http.MethodPost,
			target:     "/v1/me/onboarding/starter-egg",
			body:       []byte(`{"element":"fire"}`),
			auth:       true,
			wantStatus: http.StatusBadRequest,
			wantCode:   "idempotency_key_invalid",
		},
		{
			name:       "method",
			method:     http.MethodGet,
			target:     "/v1/me/onboarding/age-gate",
			auth:       true,
			wantStatus: http.StatusMethodNotAllowed,
			wantCode:   "method_not_allowed",
		},
		{
			name:       "query",
			method:     http.MethodPost,
			target:     "/v1/me/onboarding/starter-egg?element=fire",
			body:       []byte(`{"element":"fire"}`),
			auth:       true,
			key:        testKey,
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_query",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := onboardingRequest(
				routes,
				test.method,
				test.target,
				test.body,
				test.auth,
				test.key,
			)
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
			}
			var envelope struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if envelope.Error.Code != test.wantCode {
				t.Fatalf("code = %q, want %q", envelope.Error.Code, test.wantCode)
			}
		})
	}
}

func onboardingRoutes(
	t *testing.T,
	store Store,
	now time.Time,
) http.Handler {
	t.Helper()
	service := testService(t, store, now)
	handler, err := NewHTTPHandler(
		service,
		dojo.HeaderAuthenticator{},
		bytes.NewReader(make([]byte, 512)),
	)
	if err != nil {
		t.Fatalf("NewHTTPHandler: %v", err)
	}
	return handler.Routes()
}

func onboardingRequest(
	handler http.Handler,
	method string,
	target string,
	body []byte,
	auth bool,
	key string,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	if auth {
		request.Header.Set("X-Player-ID", testPlayerID)
	}
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}
