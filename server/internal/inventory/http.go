package inventory

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

const maxEquipRequestBody = 8 << 10

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
		return nil, errors.New("service and player authenticator are required")
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
	mux.HandleFunc("/v1/me/techniques", h.handleListTechniques)
	mux.HandleFunc("/v1/me/techniques/equip", h.handleEquipTechniques)
	mux.HandleFunc("/v1/me/loadout", h.handleCurrentLoadout)
	return mux
}

func (h *HTTPHandler) handleListTechniques(
	writer http.ResponseWriter,
	request *http.Request,
) {
	requestID := h.prepareResponse(writer)
	if request.Method != http.MethodGet {
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
	query := request.URL.Query()
	for key, values := range query {
		if (key != "limit" && key != "cursor") || len(values) != 1 {
			h.writeError(writer, requestID, apiError(
				"invalid_query",
				"query parameters are invalid for this endpoint",
				http.StatusBadRequest,
			))
			return
		}
	}
	response, err := h.service.ListTechniques(
		request.Context(),
		playerID,
		query.Get("limit"),
		query.Get("cursor"),
	)
	if err != nil {
		h.writeError(writer, requestID, err)
		return
	}
	h.writeJSON(writer, http.StatusOK, response)
}

func (h *HTTPHandler) handleEquipTechniques(
	writer http.ResponseWriter,
	request *http.Request,
) {
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
	var input EquipTechniquesRequest
	if err := decodeJSON(writer, request, &input); err != nil {
		h.writeError(writer, requestID, err)
		return
	}
	response, err := h.service.EquipTechniques(
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

func (h *HTTPHandler) handleCurrentLoadout(
	writer http.ResponseWriter,
	request *http.Request,
) {
	requestID := h.prepareResponse(writer)
	if request.Method != http.MethodGet {
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
	response, err := h.service.CurrentLoadout(request.Context(), playerID)
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
	writer.Header().Set("Cache-Control", "private, no-store")
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

func decodeJSON(
	writer http.ResponseWriter,
	request *http.Request,
	destination any,
) error {
	request.Body = http.MaxBytesReader(
		writer,
		request.Body,
		maxEquipRequestBody,
	)
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
		return apiError(
			"invalid_json",
			"request body must contain one JSON value",
			http.StatusBadRequest,
		)
	}
	return nil
}

func randomUUID(reader io.Reader) (string, error) {
	value := make([]byte, 16)
	if _, err := io.ReadFull(reader, value); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf(
		"%x-%x-%x-%x-%x",
		value[0:4],
		value[4:6],
		value[6:8],
		value[8:10],
		value[10:16],
	), nil
}
