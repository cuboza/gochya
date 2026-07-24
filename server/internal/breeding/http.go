package breeding

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gochya/gochya/server/internal/dojo"
)

const maxBreedBody = 8 << 10

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
		return nil, errors.New("breeding service and authenticator are required")
	}
	if random == nil {
		random = rand.Reader
	}
	return &HTTPHandler{
		service:       service,
		authenticator: authenticator,
		random:        random,
	}, nil
}

func (h *HTTPHandler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/breeding/breed", h.handleBreed)
	mux.HandleFunc("/v1/me/eggs", h.handleEggs)
	mux.HandleFunc("/v1/me/eggs/", h.handleEggRoute)
	return mux
}

func (h *HTTPHandler) handleBreed(writer http.ResponseWriter, request *http.Request) {
	requestID := h.prepareResponse(writer)
	if request.Method != http.MethodPost {
		h.writeError(writer, requestID, methodNotAllowedError())
		return
	}
	if request.URL.RawQuery != "" {
		h.writeError(writer, requestID, invalidQueryError())
		return
	}
	playerID, err := h.authenticator.Authenticate(request)
	if err != nil {
		h.writeError(writer, requestID, err)
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, maxBreedBody)
	var payload BreedRequest
	if err := decodeStrictJSON(request.Body, &payload); err != nil {
		h.writeError(writer, requestID, err)
		return
	}
	response, err := h.service.Breed(
		request.Context(),
		playerID,
		request.Header.Get("Idempotency-Key"),
		payload,
	)
	if err != nil {
		h.writeError(writer, requestID, err)
		return
	}
	h.writeJSON(writer, http.StatusOK, response)
}

func (h *HTTPHandler) handleEggs(writer http.ResponseWriter, request *http.Request) {
	requestID := h.prepareResponse(writer)
	if request.Method != http.MethodGet {
		h.writeError(writer, requestID, methodNotAllowedError())
		return
	}
	if request.URL.RawQuery != "" {
		h.writeError(writer, requestID, invalidQueryError())
		return
	}
	playerID, err := h.authenticator.Authenticate(request)
	if err != nil {
		h.writeError(writer, requestID, err)
		return
	}
	response, err := h.service.ListEggs(request.Context(), playerID)
	if err != nil {
		h.writeError(writer, requestID, err)
		return
	}
	h.writeJSON(writer, http.StatusOK, response)
}

func (h *HTTPHandler) handleEggRoute(writer http.ResponseWriter, request *http.Request) {
	requestID := h.prepareResponse(writer)
	tail := strings.TrimPrefix(request.URL.Path, "/v1/me/eggs/")
	parts := strings.Split(tail, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "hatch" {
		h.writeError(
			writer,
			requestID,
			apiError("route_not_found", "route was not found", http.StatusNotFound),
		)
		return
	}
	if request.Method != http.MethodPost {
		h.writeError(writer, requestID, methodNotAllowedError())
		return
	}
	if request.URL.RawQuery != "" {
		h.writeError(writer, requestID, invalidQueryError())
		return
	}
	if err := requireEmptyBody(request); err != nil {
		h.writeError(writer, requestID, err)
		return
	}
	playerID, err := h.authenticator.Authenticate(request)
	if err != nil {
		h.writeError(writer, requestID, err)
		return
	}
	response, err := h.service.Hatch(request.Context(), playerID, parts[0])
	if err != nil {
		h.writeError(writer, requestID, err)
		return
	}
	h.writeJSON(writer, http.StatusOK, response)
}

func decodeStrictJSON(reader io.Reader, target any) error {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return &Error{
			Code:       "invalid_body",
			Message:    "request body must be valid JSON",
			HTTPStatus: http.StatusBadRequest,
			Cause:      err,
		}
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return apiError(
			"invalid_body",
			"request body must contain exactly one JSON value",
			http.StatusBadRequest,
		)
	}
	return nil
}

func requireEmptyBody(request *http.Request) error {
	data, err := io.ReadAll(io.LimitReader(request.Body, 1))
	if err != nil {
		return &Error{
			Code:       "invalid_body",
			Message:    "request body could not be read",
			HTTPStatus: http.StatusBadRequest,
			Cause:      err,
		}
	}
	if len(data) != 0 {
		return apiError(
			"invalid_body",
			"request body must be empty",
			http.StatusBadRequest,
		)
	}
	return nil
}

func methodNotAllowedError() *Error {
	return apiError(
		"method_not_allowed",
		"method is not allowed",
		http.StatusMethodNotAllowed,
	)
}

func invalidQueryError() *Error {
	return apiError(
		"invalid_query",
		"query parameters are not supported by this endpoint",
		http.StatusBadRequest,
	)
}

func (h *HTTPHandler) prepareResponse(writer http.ResponseWriter) string {
	requestID, err := randomUUID(h.random)
	if err != nil {
		requestID = fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Request-ID", requestID)
	return requestID
}

func (h *HTTPHandler) writeError(
	writer http.ResponseWriter,
	requestID string,
	err error,
) {
	apiErr := asAPIError(err)
	h.writeJSON(writer, apiErr.HTTPStatus, struct {
		Error struct {
			Code      string         `json:"code"`
			Message   string         `json:"message"`
			Details   map[string]any `json:"details"`
			RequestID string         `json:"request_id"`
		} `json:"error"`
	}{
		Error: struct {
			Code      string         `json:"code"`
			Message   string         `json:"message"`
			Details   map[string]any `json:"details"`
			RequestID string         `json:"request_id"`
		}{
			Code:      apiErr.Code,
			Message:   apiErr.Message,
			Details:   map[string]any{},
			RequestID: requestID,
		},
	})
}

func (h *HTTPHandler) writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
