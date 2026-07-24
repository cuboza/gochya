package profile

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
	mux.HandleFunc("/v1/me", h.handleProfile)
	mux.HandleFunc("/v1/me/pets", h.handlePets)
	mux.HandleFunc("/v1/me/pets/", h.handlePetRoute)
	return mux
}

func (h *HTTPHandler) handleProfile(
	writer http.ResponseWriter,
	request *http.Request,
) {
	requestID := h.prepareResponse(writer)
	if request.Method != http.MethodGet {
		h.writeError(writer, requestID, methodNotAllowedError())
		return
	}
	if !validEmptyQuery(request) {
		h.writeError(writer, requestID, invalidQueryError())
		return
	}
	playerID, err := h.authenticator.Authenticate(request)
	if err != nil {
		h.writeError(writer, requestID, err)
		return
	}
	response, err := h.service.PlayerProfile(request.Context(), playerID)
	if err != nil {
		h.writeError(writer, requestID, err)
		return
	}
	h.writeJSON(writer, http.StatusOK, response)
}

func (h *HTTPHandler) handlePets(
	writer http.ResponseWriter,
	request *http.Request,
) {
	requestID := h.prepareResponse(writer)
	if request.Method != http.MethodGet {
		h.writeError(writer, requestID, methodNotAllowedError())
		return
	}
	if !validEmptyQuery(request) {
		h.writeError(writer, requestID, invalidQueryError())
		return
	}
	playerID, err := h.authenticator.Authenticate(request)
	if err != nil {
		h.writeError(writer, requestID, err)
		return
	}
	response, err := h.service.ListPets(request.Context(), playerID)
	if err != nil {
		h.writeError(writer, requestID, err)
		return
	}
	h.writeJSON(writer, http.StatusOK, response)
}

func (h *HTTPHandler) handlePetRoute(
	writer http.ResponseWriter,
	request *http.Request,
) {
	requestID := h.prepareResponse(writer)
	tail := strings.TrimPrefix(request.URL.Path, "/v1/me/pets/")
	parts := strings.Split(tail, "/")
	var activate bool
	switch {
	case len(parts) == 1 && parts[0] != "" && request.Method == http.MethodGet:
	case len(parts) == 2 && parts[0] != "" && parts[1] == "activate" &&
		request.Method == http.MethodPost:
		activate = true
	case (len(parts) == 1 && parts[0] != "") ||
		(len(parts) == 2 && parts[0] != "" && parts[1] == "activate"):
		h.writeError(writer, requestID, methodNotAllowedError())
		return
	default:
		h.writeError(writer, requestID, apiError(
			"route_not_found",
			"route was not found",
			http.StatusNotFound,
		))
		return
	}
	if !validEmptyQuery(request) {
		h.writeError(writer, requestID, invalidQueryError())
		return
	}
	if activate {
		if err := requireEmptyBody(request); err != nil {
			h.writeError(writer, requestID, err)
			return
		}
	}
	playerID, err := h.authenticator.Authenticate(request)
	if err != nil {
		h.writeError(writer, requestID, err)
		return
	}
	var response Pet
	if activate {
		response, err = h.service.ActivatePet(
			request.Context(),
			playerID,
			parts[0],
		)
	} else {
		response, err = h.service.Pet(request.Context(), playerID, parts[0])
	}
	if err != nil {
		h.writeError(writer, requestID, err)
		return
	}
	h.writeJSON(writer, http.StatusOK, response)
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

func validEmptyQuery(request *http.Request) bool {
	return request.URL.RawQuery == ""
}

func invalidQueryError() *Error {
	return apiError(
		"invalid_query",
		"query parameters are not supported by this endpoint",
		http.StatusBadRequest,
	)
}

func methodNotAllowedError() *Error {
	return apiError(
		"method_not_allowed",
		"method is not allowed",
		http.StatusMethodNotAllowed,
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
