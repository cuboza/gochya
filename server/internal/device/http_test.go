package device

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gochya/gochya/server/internal/dojo"
)

func TestHTTPDeviceEnrollmentFlow(t *testing.T) {
	now := time.Date(2026, time.July, 23, 11, 0, 0, 0, time.UTC)
	service := newTestService(
		t,
		newTestStore(),
		func(context.Context, dojo.AttestationInput) error { return nil },
		now,
	)
	handler, err := NewHTTPHandler(service, dojo.HeaderAuthenticator{}, nil)
	if err != nil {
		t.Fatalf("NewHTTPHandler: %v", err)
	}

	preflightRecorder := httptest.NewRecorder()
	preflightRequest := httptest.NewRequest(
		http.MethodPost,
		"/v1/devices/preflight",
		strings.NewReader(
			`{"deviceId":"wear-device-1","platform":"wear_os","appBuild":"100"}`,
		),
	)
	preflightRequest.Header.Set("X-Player-ID", testPlayer)
	handler.Routes().ServeHTTP(preflightRecorder, preflightRequest)
	if preflightRecorder.Code != http.StatusOK {
		t.Fatalf("preflight status = %d, body = %s", preflightRecorder.Code, preflightRecorder.Body)
	}
	if preflightRecorder.Header().Get("Cache-Control") != "no-store" ||
		preflightRecorder.Header().Get("X-Request-ID") == "" {
		t.Fatalf("preflight headers = %#v", preflightRecorder.Header())
	}
	var preflight PreflightResponse
	if err := json.Unmarshal(preflightRecorder.Body.Bytes(), &preflight); err != nil {
		t.Fatalf("decode preflight: %v", err)
	}

	registerBody, err := json.Marshal(
		signedRegisterRequest(t, testPrivateKey(7), preflight.Challenge),
	)
	if err != nil {
		t.Fatalf("encode registration: %v", err)
	}
	registerRecorder := httptest.NewRecorder()
	registerRequest := httptest.NewRequest(
		http.MethodPost,
		"/v1/devices/register",
		bytes.NewReader(registerBody),
	)
	registerRequest.Header.Set("X-Player-ID", testPlayer)
	handler.Routes().ServeHTTP(registerRecorder, registerRequest)
	if registerRecorder.Code != http.StatusOK {
		t.Fatalf("register status = %d, body = %s", registerRecorder.Code, registerRecorder.Body)
	}
	var response RegisterResponse
	if err := json.Unmarshal(registerRecorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode registration: %v", err)
	}
	if response.DeviceID != testDeviceID || !response.RegisteredAt.Equal(now) {
		t.Fatalf("registration response = %#v", response)
	}
}

func TestHTTPDeviceEnrollmentBoundaries(t *testing.T) {
	service := newTestService(
		t,
		newTestStore(),
		func(context.Context, dojo.AttestationInput) error { return nil },
		time.Now().UTC(),
	)
	handler, err := NewHTTPHandler(service, dojo.HeaderAuthenticator{}, nil)
	if err != nil {
		t.Fatalf("NewHTTPHandler: %v", err)
	}
	routes := handler.Routes()

	tests := []struct {
		name       string
		method     string
		body       string
		playerID   string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "authentication required",
			method:     http.MethodPost,
			body:       `{}`,
			wantStatus: http.StatusUnauthorized,
			wantCode:   "unauthenticated",
		},
		{
			name:       "method rejected",
			method:     http.MethodGet,
			playerID:   testPlayer,
			wantStatus: http.StatusMethodNotAllowed,
			wantCode:   "method_not_allowed",
		},
		{
			name:       "unknown field rejected",
			method:     http.MethodPost,
			body:       `{"deviceId":"wear-device-1","platform":"wear_os","appBuild":"100","extra":true}`,
			playerID:   testPlayer,
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_json",
		},
		{
			name:       "multiple values rejected",
			method:     http.MethodPost,
			body:       `{}` + "\n" + `{}`,
			playerID:   testPlayer,
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_json",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(
				test.method,
				"/v1/devices/preflight",
				strings.NewReader(test.body),
			)
			if test.playerID != "" {
				request.Header.Set("X-Player-ID", test.playerID)
			}
			routes.ServeHTTP(recorder, request)
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
			}
			var envelope struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("decode error: %v", err)
			}
			if envelope.Error.Code != test.wantCode {
				t.Fatalf("error code = %q", envelope.Error.Code)
			}
		})
	}
}
