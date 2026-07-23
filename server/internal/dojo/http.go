package dojo

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const maxRequestBody = 64 << 10

type HTTPHandler struct {
	service       *Service
	authenticator PlayerAuthenticator
	random        io.Reader
}

type PlayerAuthenticator interface {
	Authenticate(*http.Request) (string, error)
}

// HeaderAuthenticator is only a test/staging boundary. Production must inject
// an authenticator that verifies the access token before returning a player ID.
type HeaderAuthenticator struct{}

func (HeaderAuthenticator) Authenticate(request *http.Request) (string, error) {
	playerID := request.Header.Get("X-Player-ID")
	if playerID == "" {
		return "", apiError(
			"unauthenticated",
			"authenticated player is required",
			http.StatusUnauthorized,
		)
	}
	return playerID, nil
}

func NewHTTPHandler(
	service *Service,
	authenticator PlayerAuthenticator,
	random io.Reader,
) (*HTTPHandler, error) {
	if service == nil || authenticator == nil {
		return nil, errors.New("service and player authenticator are required")
	}
	if random == nil {
		random = service.random
	}
	return &HTTPHandler{service: service, authenticator: authenticator, random: random}, nil
}

func (h *HTTPHandler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/dojo/preflight", h.handlePreflight)
	mux.HandleFunc("/v1/dojo/submit", h.handleSubmit)
	return mux
}

func (h *HTTPHandler) handlePreflight(writer http.ResponseWriter, request *http.Request) {
	requestID := h.prepareResponse(writer)
	if request.Method != http.MethodPost {
		h.writeError(writer, requestID, apiError(
			"method_not_allowed",
			"method is not allowed",
			http.StatusMethodNotAllowed,
		))
		return
	}
	playerID, err := h.authenticator.Authenticate(request)
	if err != nil {
		h.writeError(writer, requestID, err)
		return
	}
	var input PreflightRequest
	if err := decodeJSON(writer, request, &input); err != nil {
		h.writeError(writer, requestID, err)
		return
	}
	response, err := h.service.Preflight(request.Context(), playerID, input)
	if err != nil {
		h.writeError(writer, requestID, err)
		return
	}
	h.writeJSON(writer, http.StatusOK, response)
}

func (h *HTTPHandler) handleSubmit(writer http.ResponseWriter, request *http.Request) {
	requestID := h.prepareResponse(writer)
	if request.Method != http.MethodPost {
		h.writeError(writer, requestID, apiError(
			"method_not_allowed",
			"method is not allowed",
			http.StatusMethodNotAllowed,
		))
		return
	}
	playerID, err := h.authenticator.Authenticate(request)
	if err != nil {
		h.writeError(writer, requestID, err)
		return
	}
	var input SubmitRequest
	if err := decodeJSON(writer, request, &input); err != nil {
		h.writeError(writer, requestID, err)
		return
	}
	response, err := h.service.Submit(
		request.Context(),
		playerID,
		request.Header.Get("Idempotency-Key"),
		input,
	)
	if err != nil {
		h.writeError(writer, requestID, err)
		return
	}
	h.writeJSON(writer, http.StatusOK, response)
}

func (h *HTTPHandler) prepareResponse(writer http.ResponseWriter) string {
	requestID, err := randomUUID(h.random)
	if err != nil {
		requestID = fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("X-Request-ID", requestID)
	return requestID
}

func (h *HTTPHandler) writeError(writer http.ResponseWriter, requestID string, err error) {
	apiErr := asAPIError(err)
	details := apiErr.Details
	if details == nil {
		details = map[string]any{}
	}
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
			Details:   details,
			RequestID: requestID,
		},
	})
}

func (h *HTTPHandler) writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func decodeJSON(writer http.ResponseWriter, request *http.Request, destination any) error {
	request.Body = http.MaxBytesReader(writer, request.Body, maxRequestBody)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return &Error{
			Code:       "invalid_json",
			Message:    "request body is not valid for this endpoint",
			HTTPStatus: http.StatusBadRequest,
			Cause:      err,
		}
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return apiError("invalid_json", "request body must contain one JSON value", http.StatusBadRequest)
	}
	return nil
}
