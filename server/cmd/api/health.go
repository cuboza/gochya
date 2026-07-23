package main

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

const readinessTimeout = time.Second

type databasePinger interface {
	Ping(context.Context) error
}

func newRootHandler(api http.Handler, database databasePinger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health/live", healthMethod(liveness))
	mux.HandleFunc("/health/ready", healthMethod(readiness(database)))
	mux.Handle("/", api)
	return mux
}

func healthMethod(next http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			writer.Header().Set("Allow", "GET, HEAD")
			writeHealth(writer, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		next(writer, request)
	}
}

func liveness(writer http.ResponseWriter, _ *http.Request) {
	writeHealth(writer, http.StatusOK, "ok")
}

func readiness(database databasePinger) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		ctx, cancel := context.WithTimeout(request.Context(), readinessTimeout)
		defer cancel()
		if err := database.Ping(ctx); err != nil {
			writeHealth(writer, http.StatusServiceUnavailable, "unavailable")
			return
		}
		writeHealth(writer, http.StatusOK, "ok")
	}
}

func writeHealth(writer http.ResponseWriter, status int, value string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(struct {
		Status string `json:"status"`
	}{Status: value})
}
