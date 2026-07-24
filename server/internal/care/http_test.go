package care

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gochya/gochya/server/internal/dojo"
)

func TestHTTPCareSync(t *testing.T) {
	now := time.Date(2026, time.July, 24, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{
		response: SyncResponse{
			Results: []CommandResult{{
				OperationID: testOpID,
				Status:      StatusApplied,
			}},
			NewRevision: 1,
			ServerTime:  now,
		},
	}
	handler := careRoutes(t, store, now)
	body, _ := json.Marshal(validSyncRequest(now))
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/sync/commands",
		bytes.NewReader(body),
	)
	request.Header.Set("X-Player-ID", testPlayerID)
	request.Header.Set("If-Match", "0")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body)
	}
	var decoded SyncResponse
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if decoded.NewRevision != 1 ||
		len(decoded.Results) != 1 ||
		decoded.Results[0].Status != StatusApplied {
		t.Fatalf("response = %#v", decoded)
	}
	if response.Header().Get("Cache-Control") != "private, no-store" ||
		response.Header().Get("X-Request-ID") == "" {
		t.Fatalf("headers = %#v", response.Header())
	}
}

func TestHTTPCareBoundaries(t *testing.T) {
	now := time.Date(2026, time.July, 24, 12, 0, 0, 0, time.UTC)
	validBody, _ := json.Marshal(validSyncRequest(now))
	tests := []struct {
		name       string
		method     string
		target     string
		body       []byte
		auth       bool
		ifMatch    string
		wantStatus int
		wantCode   string
	}{
		{"auth", http.MethodPost, "/v1/sync/commands", validBody, false, "0", 401, "unauthenticated"},
		{"method", http.MethodGet, "/v1/sync/commands", nil, true, "0", 405, "method_not_allowed"},
		{"query", http.MethodPost, "/v1/sync/commands?x=1", validBody, true, "0", 400, "invalid_query"},
		{"body", http.MethodPost, "/v1/sync/commands", []byte(`{"deviceId":"x","commands":[],"extra":1}`), true, "0", 400, "invalid_body"},
		{"multiple JSON", http.MethodPost, "/v1/sync/commands", append(validBody, []byte(` {}`)...), true, "0", 400, "invalid_body"},
		{"if match", http.MethodPost, "/v1/sync/commands", validBody, true, "", 400, "if_match_invalid"},
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
			request.Header.Set("If-Match", test.ifMatch)
			response := httptest.NewRecorder()
			careRoutes(t, &fakeStore{}, now).ServeHTTP(response, request)
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
			if envelope.Error.Code != test.wantCode {
				t.Fatalf("code = %q, want %q", envelope.Error.Code, test.wantCode)
			}
		})
	}
}

func careRoutes(t *testing.T, store Store, now time.Time) http.Handler {
	t.Helper()
	service := careService(t, store, now)
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
