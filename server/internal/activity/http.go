package activity

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gochya/gochya/server/internal/dojo"
)

const maxSyncBody = 32 << 10

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
		return nil, errors.New("activity service and authenticator are required")
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
	mux.HandleFunc("/v1/sync/activity", h.sync)
	mux.HandleFunc("/v1/me/activity/week", h.week)
	mux.HandleFunc("/v1/me/activity/reward", h.reward)
	return mux
}

func (h *HTTPHandler) sync(writer http.ResponseWriter, request *http.Request) {
	requestID := h.prepare(writer)
	if request.Method != http.MethodPost {
		h.writeError(
			writer,
			requestID,
			apiError("method_not_allowed", "method is not allowed", http.StatusMethodNotAllowed),
		)
		return
	}
	if request.URL.RawQuery != "" {
		h.writeError(
			writer,
			requestID,
			apiError("invalid_query", "query parameters are invalid", http.StatusBadRequest),
		)
		return
	}
	playerID, err := h.authenticator.Authenticate(request)
	if err != nil {
		h.writeError(writer, requestID, err)
		return
	}
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, maxSyncBody))
	decoder.DisallowUnknownFields()
	var input SyncRequest
	if err := decoder.Decode(&input); err != nil {
		h.writeError(
			writer,
			requestID,
			apiError("invalid_json", "request body is invalid", http.StatusBadRequest),
		)
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		h.writeError(
			writer,
			requestID,
			apiError("invalid_json", "one JSON value is required", http.StatusBadRequest),
		)
		return
	}
	response, err := h.service.Sync(request.Context(), playerID, input)
	if err != nil {
		h.writeError(writer, requestID, err)
		return
	}
	h.writeJSON(writer, http.StatusOK, response)
}

func (h *HTTPHandler) week(writer http.ResponseWriter, request *http.Request) {
	requestID := h.prepare(writer)
	if request.Method != http.MethodGet {
		h.writeError(
			writer,
			requestID,
			apiError("method_not_allowed", "method is not allowed", http.StatusMethodNotAllowed),
		)
		return
	}
	if request.URL.RawQuery != "" {
		h.writeError(
			writer,
			requestID,
			apiError("invalid_query", "query parameters are invalid", http.StatusBadRequest),
		)
		return
	}
	playerID, err := h.authenticator.Authenticate(request)
	if err != nil {
		h.writeError(writer, requestID, err)
		return
	}
	response, err := h.service.Week(request.Context(), playerID)
	if err != nil {
		h.writeError(writer, requestID, err)
		return
	}
	h.writeJSON(writer, http.StatusOK, response)
}

func (h *HTTPHandler) reward(writer http.ResponseWriter, request *http.Request) {
	requestID := h.prepare(writer)
	if request.Method != http.MethodPost {
		h.writeError(
			writer,
			requestID,
			apiError("method_not_allowed", "method is not allowed", http.StatusMethodNotAllowed),
		)
		return
	}
	if request.URL.RawQuery != "" {
		h.writeError(
			writer,
			requestID,
			apiError("invalid_query", "query parameters are invalid", http.StatusBadRequest),
		)
		return
	}
	playerID, err := h.authenticator.Authenticate(request)
	if err != nil {
		h.writeError(writer, requestID, err)
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, 1)
	buffer := make([]byte, 1)
	if count, err := request.Body.Read(buffer); err != nil && !errors.Is(err, io.EOF) ||
		count != 0 {
		h.writeError(
			writer,
			requestID,
			apiError("invalid_body", "request body must be empty", http.StatusBadRequest),
		)
		return
	}
	response, err := h.service.ClaimReward(request.Context(), playerID)
	if err != nil {
		h.writeError(writer, requestID, err)
		return
	}
	h.writeJSON(writer, http.StatusOK, response)
}

func (h *HTTPHandler) prepare(writer http.ResponseWriter) string {
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
	h.writeJSON(writer, apiErr.HTTPStatus, map[string]any{
		"error": map[string]any{
			"code":       apiErr.Code,
			"message":    apiErr.Message,
			"details":    map[string]any{},
			"request_id": requestID,
		},
	})
}

func (h *HTTPHandler) writeJSON(
	writer http.ResponseWriter,
	status int,
	value any,
) {
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func randomUUID(reader io.Reader) (string, error) {
	value := make([]byte, 16)
	if _, err := io.ReadFull(reader, value); err != nil {
		return "", err
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	return fmt.Sprintf(
		"%x-%x-%x-%x-%x",
		value[:4],
		value[4:6],
		value[6:8],
		value[8:10],
		value[10:],
	), nil
}
