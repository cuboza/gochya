package shop

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gochya/gochya/server/internal/dojo"
)

const maxPurchaseBody = 8 << 10

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
		return nil, errors.New("shop service and authenticator are required")
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
	mux.HandleFunc("/v1/shop", h.handleCatalog)
	mux.HandleFunc("/v1/shop/buy", h.handlePurchase)
	mux.HandleFunc("/v1/me/items", h.handleInventory)
	return mux
}

func (h *HTTPHandler) handleCatalog(writer http.ResponseWriter, request *http.Request) {
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
	response, err := h.service.Catalog(request.Context(), playerID)
	if err != nil {
		h.writeError(writer, requestID, err)
		return
	}
	h.writeJSON(writer, http.StatusOK, response)
}

func (h *HTTPHandler) handlePurchase(writer http.ResponseWriter, request *http.Request) {
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
	request.Body = http.MaxBytesReader(writer, request.Body, maxPurchaseBody)
	var payload PurchaseRequest
	if err := decodeStrictJSON(request.Body, &payload); err != nil {
		h.writeError(writer, requestID, err)
		return
	}
	response, err := h.service.Purchase(
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

func (h *HTTPHandler) handleInventory(writer http.ResponseWriter, request *http.Request) {
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
	response, err := h.service.Inventory(request.Context(), playerID)
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

func methodNotAllowedError() error {
	return apiError("method_not_allowed", "method is not allowed", http.StatusMethodNotAllowed)
}

func invalidQueryError() error {
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

func (h *HTTPHandler) writeError(writer http.ResponseWriter, requestID string, err error) {
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

func randomUUID(reader io.Reader) (string, error) {
	value := make([]byte, 16)
	if _, err := io.ReadFull(reader, value); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(value)
	return encoded[0:8] + "-" +
		encoded[8:12] + "-" +
		encoded[12:16] + "-" +
		encoded[16:20] + "-" +
		encoded[20:32], nil
}
