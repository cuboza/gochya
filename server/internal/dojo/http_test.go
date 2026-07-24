package dojo

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPRejectsUnknownRawSignalFields(t *testing.T) {
	fixture := newFixture(t)
	handler, err := NewHTTPHandler(fixture.service, HeaderAuthenticator{}, nil)
	if err != nil {
		t.Fatalf("NewHTTPHandler: %v", err)
	}
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/dojo/preflight",
		strings.NewReader(`{"deviceId":"watch-1","appBuild":"100","rawSamples":[1,2,3]}`),
	)
	request.Header.Set("X-Player-ID", testPlayer)
	response := httptest.NewRecorder()

	handler.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body)
	}
	var envelope struct {
		Error struct {
			Code      string         `json:"code"`
			Details   map[string]any `json:"details"`
			RequestID string         `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error.Code != "invalid_json" || envelope.Error.RequestID == "" ||
		envelope.Error.Details == nil {
		t.Fatalf("error envelope = %#v", envelope)
	}
	if response.Header().Get("X-Request-ID") != envelope.Error.RequestID {
		t.Fatal("header and body request IDs differ")
	}
}

func TestHTTPSuccessDoesNotEchoPrivateEvidence(t *testing.T) {
	fixture := newFixture(t)
	handler, err := NewHTTPHandler(fixture.service, HeaderAuthenticator{}, nil)
	if err != nil {
		t.Fatalf("NewHTTPHandler: %v", err)
	}
	input := fixture.request(t, fixture.preflight(t))
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/dojo/submit", bytes.NewReader(body))
	request.Header.Set("X-Player-ID", testPlayer)
	request.Header.Set("Idempotency-Key", "00000000-0000-4000-8000-000000000011")
	response := httptest.NewRecorder()

	handler.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body)
	}
	var success SubmitResponse
	if err := json.Unmarshal(response.Body.Bytes(), &success); err != nil {
		t.Fatalf("decode success: %v", err)
	}
	if success.TraceID == "" ||
		response.Header().Get("X-Trace-ID") != success.TraceID {
		t.Fatalf(
			"trace header = %q, response trace = %q",
			response.Header().Get("X-Trace-ID"),
			success.TraceID,
		)
	}
	output := response.Body.String()
	for _, forbidden := range []string{
		"featureSummary",
		"metrics",
		"heartEvidence",
		"sampleCount",
		"payloadSignature",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("response contains private evidence field %q: %s", forbidden, output)
		}
	}
}

func TestHTTPPreflightReturnsFlowTraceID(t *testing.T) {
	fixture := newFixture(t)
	handler, err := NewHTTPHandler(fixture.service, HeaderAuthenticator{}, nil)
	if err != nil {
		t.Fatalf("NewHTTPHandler: %v", err)
	}
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/dojo/preflight",
		strings.NewReader(`{"deviceId":"watch-1","appBuild":"100"}`),
	)
	request.Header.Set("X-Player-ID", testPlayer)
	response := httptest.NewRecorder()

	handler.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body)
	}
	var preflight PreflightResponse
	if err := json.Unmarshal(response.Body.Bytes(), &preflight); err != nil {
		t.Fatalf("decode preflight: %v", err)
	}
	if preflight.TraceID == "" ||
		response.Header().Get("X-Trace-ID") != preflight.TraceID {
		t.Fatalf(
			"trace header = %q, response trace = %q",
			response.Header().Get("X-Trace-ID"),
			preflight.TraceID,
		)
	}
}

func TestHTTPRequiresAuthenticatedPlayer(t *testing.T) {
	fixture := newFixture(t)
	handler, err := NewHTTPHandler(fixture.service, HeaderAuthenticator{}, nil)
	if err != nil {
		t.Fatalf("NewHTTPHandler: %v", err)
	}
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/dojo/preflight",
		strings.NewReader(`{"deviceId":"watch-1","appBuild":"100"}`),
	)
	response := httptest.NewRecorder()

	handler.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body)
	}
}

func TestHTTPFailureReturnsFlowTraceID(t *testing.T) {
	fixture := newFixture(t)
	handler, err := NewHTTPHandler(fixture.service, HeaderAuthenticator{}, nil)
	if err != nil {
		t.Fatalf("NewHTTPHandler: %v", err)
	}
	preflight := fixture.preflight(t)
	input := fixture.request(t, preflight)
	input.Metrics.Precision = 0.1
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/dojo/submit",
		bytes.NewReader(body),
	)
	request.Header.Set("X-Player-ID", testPlayer)
	request.Header.Set("Idempotency-Key", "00000000-0000-4000-8000-000000000042")
	response := httptest.NewRecorder()

	handler.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body)
	}
	if response.Header().Get("X-Trace-ID") != preflight.TraceID {
		t.Fatalf(
			"trace header = %q, want %q",
			response.Header().Get("X-Trace-ID"),
			preflight.TraceID,
		)
	}
}
