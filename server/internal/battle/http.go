package battle

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gochya/gochya/server/internal/dojo"
)

const maxQueueBody = 4 << 10

type HTTPHandler struct {
	service       *Service
	authenticator dojo.PlayerAuthenticator
	random        io.Reader
}

func NewHTTPHandler(
	service *Service,
	authenticator dojo.PlayerAuthenticator,
	random io.Reader,
) (*HTTPHandler, error) {
	if service == nil || authenticator == nil {
		return nil, errors.New("battle service and authenticator are required")
	}
	if random == nil {
		random = rand.Reader
	}
	return &HTTPHandler{service: service, authenticator: authenticator, random: random}, nil
}

func (h *HTTPHandler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/matchmaking/queue", h.queue)
	mux.HandleFunc("/v1/match/", h.match)
	mux.HandleFunc("/v1/me/matches/history", h.history)
	return mux
}

func (h *HTTPHandler) queue(w http.ResponseWriter, r *http.Request) {
	requestID := h.prepare(w)
	if r.Method != http.MethodPost {
		h.writeError(w, requestID, apiError("method_not_allowed", "method is not allowed", 405))
		return
	}
	playerID, err := h.authenticator.Authenticate(r)
	if err != nil {
		h.writeError(w, requestID, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxQueueBody)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var input QueueRequest
	if err := decoder.Decode(&input); err != nil {
		h.writeError(w, requestID, apiError("invalid_json", "request body is invalid", 400))
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		h.writeError(w, requestID, apiError("invalid_json", "one JSON value is required", 400))
		return
	}
	response, err := h.service.Queue(r.Context(), playerID, r.Header.Get("Idempotency-Key"), input)
	if err != nil {
		h.writeError(w, requestID, err)
		return
	}
	h.writeJSON(w, http.StatusOK, response)
}

func (h *HTTPHandler) match(w http.ResponseWriter, r *http.Request) {
	requestID := h.prepare(w)
	path := strings.TrimPrefix(r.URL.Path, "/v1/match/")
	segments := strings.Split(path, "/")
	if path == "" || segments[0] == "" || len(segments) > 2 ||
		(len(segments) == 2 && segments[1] != "confirm") {
		h.writeError(w, requestID, apiError("route_not_found", "route was not found", 404))
		return
	}
	if r.URL.RawQuery != "" {
		h.writeError(w, requestID, apiError("invalid_query", "query parameters are invalid", 400))
		return
	}
	if len(segments) == 1 {
		if r.Method != http.MethodGet {
			h.writeError(w, requestID, apiError("method_not_allowed", "method is not allowed", 405))
			return
		}
		h.matchReplay(w, r, requestID, segments[0])
		return
	}
	if r.Method != http.MethodPost {
		h.writeError(w, requestID, apiError("method_not_allowed", "method is not allowed", 405))
		return
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1))
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		h.writeError(w, requestID, apiError("invalid_body", "request body must be empty", 400))
		return
	}
	playerID, err := h.authenticator.Authenticate(r)
	if err != nil {
		h.writeError(w, requestID, err)
		return
	}
	response, err := h.service.Confirm(r.Context(), playerID, segments[0])
	if err != nil {
		h.writeError(w, requestID, err)
		return
	}
	h.writeJSON(w, http.StatusOK, response)
}

func (h *HTTPHandler) matchReplay(
	w http.ResponseWriter,
	r *http.Request,
	requestID string,
	matchID string,
) {
	playerID, err := h.authenticator.Authenticate(r)
	if err != nil {
		h.writeError(w, requestID, err)
		return
	}
	response, err := h.service.Match(r.Context(), playerID, matchID)
	if err != nil {
		h.writeError(w, requestID, err)
		return
	}
	h.writeJSON(w, http.StatusOK, response)
}

func (h *HTTPHandler) history(w http.ResponseWriter, r *http.Request) {
	requestID := h.prepare(w)
	if r.Method != http.MethodGet {
		h.writeError(w, requestID, apiError("method_not_allowed", "method is not allowed", 405))
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(query) > 1 || len(query["limit"]) > 1 {
		h.writeError(w, requestID, apiError("invalid_query", "query parameters are invalid", 400))
		return
	}
	for key := range query {
		if key != "limit" {
			h.writeError(w, requestID, apiError("invalid_query", "query parameters are invalid", 400))
			return
		}
	}
	playerID, err := h.authenticator.Authenticate(r)
	if err != nil {
		h.writeError(w, requestID, err)
		return
	}
	response, err := h.service.History(r.Context(), playerID, query.Get("limit"))
	if err != nil {
		h.writeError(w, requestID, err)
		return
	}
	h.writeJSON(w, http.StatusOK, response)
}

func (h *HTTPHandler) prepare(w http.ResponseWriter) string {
	requestID, err := randomUUID(h.random)
	if err != nil {
		requestID = fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Request-ID", requestID)
	return requestID
}

func (h *HTTPHandler) writeError(w http.ResponseWriter, requestID string, err error) {
	apiErr := asAPIError(err)
	h.writeJSON(w, apiErr.HTTPStatus, map[string]any{"error": map[string]any{
		"code": apiErr.Code, "message": apiErr.Message,
		"details": map[string]any{}, "request_id": requestID,
	}})
}

func (h *HTTPHandler) writeJSON(w http.ResponseWriter, status int, value any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
