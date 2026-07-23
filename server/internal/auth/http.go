package auth

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

const maxAuthRequestBody = 4 << 10

type HTTPHandler struct {
	sessions SessionManager
	random   io.Reader
}

type refreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

func NewHTTPHandler(
	sessions SessionManager,
	random io.Reader,
) (*HTTPHandler, error) {
	if sessions == nil {
		return nil, errors.New("session manager is required")
	}
	if random == nil {
		random = rand.Reader
	}
	return &HTTPHandler{sessions: sessions, random: random}, nil
}

func (h *HTTPHandler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/auth/refresh", h.handleRefresh)
	mux.HandleFunc("/v1/auth/logout", h.handleLogout)
	return mux
}

func (h *HTTPHandler) handleRefresh(
	writer http.ResponseWriter,
	request *http.Request,
) {
	requestID := h.prepareResponse(writer)
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		h.writeError(
			writer,
			requestID,
			"method_not_allowed",
			"method is not allowed",
			http.StatusMethodNotAllowed,
		)
		return
	}
	var input refreshRequest
	if err := decodeRequest(writer, request, &input); err != nil {
		h.writeError(
			writer,
			requestID,
			"invalid_json",
			"request body is not valid for this endpoint",
			http.StatusBadRequest,
		)
		return
	}
	pair, err := h.sessions.Refresh(request.Context(), input.RefreshToken)
	if err != nil {
		if errors.Is(err, ErrRefreshTokenInvalid) ||
			errors.Is(err, ErrRefreshTokenReused) {
			h.writeError(
				writer,
				requestID,
				"refresh_token_invalid",
				"refresh token is invalid or expired",
				http.StatusUnauthorized,
			)
			return
		}
		h.writeError(
			writer,
			requestID,
			"internal_error",
			"internal server error",
			http.StatusInternalServerError,
		)
		return
	}
	h.writeJSON(writer, http.StatusOK, pair)
}

func (h *HTTPHandler) handleLogout(
	writer http.ResponseWriter,
	request *http.Request,
) {
	requestID := h.prepareResponse(writer)
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		h.writeError(
			writer,
			requestID,
			"method_not_allowed",
			"method is not allowed",
			http.StatusMethodNotAllowed,
		)
		return
	}
	var input refreshRequest
	if err := decodeRequest(writer, request, &input); err != nil {
		h.writeError(
			writer,
			requestID,
			"invalid_json",
			"request body is not valid for this endpoint",
			http.StatusBadRequest,
		)
		return
	}
	if err := h.sessions.Logout(request.Context(), input.RefreshToken); err != nil {
		h.writeError(
			writer,
			requestID,
			"internal_error",
			"internal server error",
			http.StatusInternalServerError,
		)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (h *HTTPHandler) prepareResponse(writer http.ResponseWriter) string {
	requestID, err := randomUUID(h.random)
	if err != nil {
		requestID = "request-id-unavailable"
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Request-ID", requestID)
	return requestID
}

func (h *HTTPHandler) writeError(
	writer http.ResponseWriter,
	requestID string,
	code string,
	message string,
	status int,
) {
	h.writeJSON(writer, status, struct {
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
			Code:      code,
			Message:   message,
			Details:   map[string]any{},
			RequestID: requestID,
		},
	})
}

func (h *HTTPHandler) writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func decodeRequest(
	writer http.ResponseWriter,
	request *http.Request,
	destination any,
) error {
	request.Body = http.MaxBytesReader(writer, request.Body, maxAuthRequestBody)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("request body must contain one JSON value")
	}
	return nil
}
