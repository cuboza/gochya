package activity

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gochya/gochya/server/internal/dojo"
)

func TestHTTPActivitySync(t *testing.T) {
	now := time.Date(2026, time.July, 24, 8, 30, 0, 0, time.UTC)
	expected := SyncResponse{
		Date:             "2026-07-24",
		Vitality:         104,
		VitalityDelta:    104,
		StatGains:        StatGains{Strength: 7, Agility: 7, Endurance: 12, Focus: 7},
		StatGainDeltas:   StatDeltas{Strength: 7, Agility: 7, Endurance: 12, Focus: 7},
		Goals:            Goals{Steps: 10_000, SleepHours: 8, ActiveCalories: 500},
		SnapshotAccepted: true,
	}
	store := &fakeStore{response: expected}
	routes := activityRoutes(t, store, now)
	body, err := json.Marshal(validSyncRequest(now))
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/sync/activity",
		bytes.NewReader(body),
	)
	request.Header.Set("X-Player-ID", activityPlayerID)
	recorder := httptest.NewRecorder()

	routes.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	var response SyncResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response != expected || len(store.commits) != 1 {
		t.Fatalf("response/commits = %#v/%d", response, len(store.commits))
	}
	if recorder.Header().Get("Cache-Control") != "private, no-store" ||
		recorder.Header().Get("X-Request-ID") == "" {
		t.Fatalf("headers = %#v", recorder.Header())
	}
}

func TestHTTPActivitySyncBoundaries(t *testing.T) {
	now := time.Date(2026, time.July, 24, 8, 30, 0, 0, time.UTC)
	validBody, err := json.Marshal(validSyncRequest(now))
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	tests := []struct {
		name       string
		method     string
		target     string
		body       []byte
		auth       bool
		wantStatus int
		wantCode   string
	}{
		{
			name:       "authentication required",
			method:     http.MethodPost,
			target:     "/v1/sync/activity",
			body:       validBody,
			wantStatus: http.StatusUnauthorized,
			wantCode:   "unauthenticated",
		},
		{
			name:       "method rejected",
			method:     http.MethodGet,
			target:     "/v1/sync/activity",
			auth:       true,
			wantStatus: http.StatusMethodNotAllowed,
			wantCode:   "method_not_allowed",
		},
		{
			name:       "query rejected",
			method:     http.MethodPost,
			target:     "/v1/sync/activity?debug=true",
			body:       validBody,
			auth:       true,
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_query",
		},
		{
			name:       "invalid JSON",
			method:     http.MethodPost,
			target:     "/v1/sync/activity",
			body:       []byte(`{"snapshot":`),
			auth:       true,
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_json",
		},
		{
			name:       "unknown field",
			method:     http.MethodPost,
			target:     "/v1/sync/activity",
			body:       []byte(`{"snapshot":{},"sourceMetadata":"healthkit://watch","award":999}`),
			auth:       true,
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_json",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			routes := activityRoutes(t, &fakeStore{}, now)
			request := httptest.NewRequest(
				test.method,
				test.target,
				bytes.NewReader(test.body),
			)
			if test.auth {
				request.Header.Set("X-Player-ID", activityPlayerID)
			}
			recorder := httptest.NewRecorder()
			routes.ServeHTTP(recorder, request)
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
			}
			var response struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode error: %v", err)
			}
			if response.Error.Code != test.wantCode {
				t.Fatalf("code = %q, want %q", response.Error.Code, test.wantCode)
			}
		})
	}
}

func activityRoutes(
	t *testing.T,
	store Store,
	now time.Time,
) http.Handler {
	t.Helper()
	service, err := NewService(ServiceConfig{
		Store: store,
		Core:  fakeCore{},
		Now:   func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	handler, err := NewHTTPHandler(
		service,
		dojo.HeaderAuthenticator{},
		bytes.NewReader(make([]byte, 16)),
	)
	if err != nil {
		t.Fatalf("NewHTTPHandler: %v", err)
	}
	return handler.Routes()
}
