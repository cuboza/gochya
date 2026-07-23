package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeDatabasePinger struct {
	err   error
	calls int
}

func (pinger *fakeDatabasePinger) Ping(context.Context) error {
	pinger.calls++
	return pinger.err
}

func TestHealthEndpoints(t *testing.T) {
	database := &fakeDatabasePinger{}
	fallback := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusTeapot)
	})
	handler := newRootHandler(fallback, database)

	live := httptest.NewRecorder()
	handler.ServeHTTP(live, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	if live.Code != http.StatusOK || live.Body.String() != "{\"status\":\"ok\"}\n" {
		t.Fatalf("liveness = %d %q", live.Code, live.Body.String())
	}
	if database.calls != 0 {
		t.Fatalf("liveness database calls = %d", database.calls)
	}

	ready := httptest.NewRecorder()
	handler.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if ready.Code != http.StatusOK || database.calls != 1 {
		t.Fatalf("readiness = %d, database calls = %d", ready.Code, database.calls)
	}

	fallbackResponse := httptest.NewRecorder()
	handler.ServeHTTP(
		fallbackResponse,
		httptest.NewRequest(http.MethodGet, "/v1/dojo/preflight", nil),
	)
	if fallbackResponse.Code != http.StatusTeapot {
		t.Fatalf("fallback status = %d", fallbackResponse.Code)
	}
}

func TestReadinessFailsClosed(t *testing.T) {
	database := &fakeDatabasePinger{err: errors.New("database unavailable")}
	handler := newRootHandler(http.NotFoundHandler(), database)
	response := httptest.NewRecorder()
	handler.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/health/ready", nil),
	)
	if response.Code != http.StatusServiceUnavailable ||
		response.Body.String() != "{\"status\":\"unavailable\"}\n" {
		t.Fatalf("readiness = %d %q", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", response.Header().Get("Cache-Control"))
	}
}

func TestHealthRejectsOtherMethods(t *testing.T) {
	handler := newRootHandler(http.NotFoundHandler(), &fakeDatabasePinger{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodPost, "/health/live", nil),
	)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", response.Code)
	}
	if response.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("Allow = %q", response.Header().Get("Allow"))
	}
}
