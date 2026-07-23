package auth

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

const maxAuthRequestBody = 24 << 10

type HTTPHandler struct {
	sessions SessionManager
	google   GoogleExchanger
	apple    AppleExchanger
	samsung  SamsungExchanger
	random   io.Reader
}

type HTTPHandlerConfig struct {
	Sessions SessionManager
	Google   GoogleExchanger
	Apple    AppleExchanger
	Samsung  SamsungExchanger
	Random   io.Reader
}

type refreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

type googleLoginRequest struct {
	IDToken  string `json:"idToken"`
	DeviceID string `json:"deviceId,omitempty"`
}

type appleLoginRequest struct {
	IdentityToken string `json:"identityToken"`
	Nonce         string `json:"nonce"`
	DeviceID      string `json:"deviceId,omitempty"`
}

type samsungPreflightRequest struct {
	RedirectURI string `json:"redirectUri"`
}

type samsungLoginRequest struct {
	Code         string `json:"code"`
	State        string `json:"state"`
	Nonce        string `json:"nonce"`
	CodeVerifier string `json:"codeVerifier"`
	RedirectURI  string `json:"redirectUri"`
	DeviceID     string `json:"deviceId,omitempty"`
}

func NewHTTPHandler(
	sessions SessionManager,
	random io.Reader,
) (*HTTPHandler, error) {
	return newHTTPHandler(sessions, nil, nil, nil, random)
}

func NewHTTPHandlerWithGoogle(
	sessions SessionManager,
	google GoogleExchanger,
	random io.Reader,
) (*HTTPHandler, error) {
	if google == nil {
		return nil, errors.New("Google exchanger is required")
	}
	return newHTTPHandler(sessions, google, nil, nil, random)
}

func NewHTTPHandlerWithProviders(
	config HTTPHandlerConfig,
) (*HTTPHandler, error) {
	if config.Google == nil && config.Apple == nil && config.Samsung == nil {
		return nil, errors.New("at least one identity provider is required")
	}
	return newHTTPHandler(
		config.Sessions,
		config.Google,
		config.Apple,
		config.Samsung,
		config.Random,
	)
}

func newHTTPHandler(
	sessions SessionManager,
	google GoogleExchanger,
	apple AppleExchanger,
	samsung SamsungExchanger,
	random io.Reader,
) (*HTTPHandler, error) {
	if sessions == nil {
		return nil, errors.New("session manager is required")
	}
	if random == nil {
		random = rand.Reader
	}
	return &HTTPHandler{
		sessions: sessions,
		google:   google,
		apple:    apple,
		samsung:  samsung,
		random:   random,
	}, nil
}

func (h *HTTPHandler) Routes() http.Handler {
	mux := http.NewServeMux()
	if h.google != nil {
		mux.HandleFunc("/v1/auth/google", h.handleGoogle)
	}
	if h.apple != nil {
		mux.HandleFunc("/v1/auth/apple/preflight", h.handleApplePreflight)
		mux.HandleFunc("/v1/auth/apple", h.handleApple)
	}
	if h.samsung != nil {
		mux.HandleFunc("/v1/auth/samsung/preflight", h.handleSamsungPreflight)
		mux.HandleFunc("/v1/auth/samsung", h.handleSamsung)
	}
	mux.HandleFunc("/v1/auth/refresh", h.handleRefresh)
	mux.HandleFunc("/v1/auth/logout", h.handleLogout)
	return mux
}

func (h *HTTPHandler) handleGoogle(
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
	var input googleLoginRequest
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
	response, err := h.google.Exchange(
		request.Context(),
		input.IDToken,
		input.DeviceID,
	)
	if err != nil {
		var unavailable *IdentityProviderUnavailableError
		switch {
		case errors.Is(err, ErrLoginRequestInvalid):
			h.writeError(
				writer,
				requestID,
				"login_request_invalid",
				"login request is invalid",
				http.StatusBadRequest,
			)
		case errors.Is(err, ErrIdentityTokenInvalid):
			h.writeError(
				writer,
				requestID,
				"identity_token_invalid",
				"Google identity token is invalid",
				http.StatusUnauthorized,
			)
		case errors.As(err, &unavailable):
			h.writeError(
				writer,
				requestID,
				"identity_provider_unavailable",
				"Google identity verification is temporarily unavailable",
				http.StatusServiceUnavailable,
			)
		default:
			h.writeError(
				writer,
				requestID,
				"internal_error",
				"internal server error",
				http.StatusInternalServerError,
			)
		}
		return
	}
	h.writeJSON(writer, http.StatusOK, response)
}

func (h *HTTPHandler) handleApplePreflight(
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
	if err := requireEmptyRequestBody(writer, request); err != nil {
		h.writeError(
			writer,
			requestID,
			"invalid_json",
			"request body must be empty",
			http.StatusBadRequest,
		)
		return
	}
	response, err := h.apple.Preflight(request.Context())
	if err != nil {
		h.writeError(
			writer,
			requestID,
			"internal_error",
			"internal server error",
			http.StatusInternalServerError,
		)
		return
	}
	h.writeJSON(writer, http.StatusOK, response)
}

func (h *HTTPHandler) handleApple(
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
	var input appleLoginRequest
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
	response, err := h.apple.Exchange(
		request.Context(),
		input.IdentityToken,
		input.Nonce,
		input.DeviceID,
	)
	if err != nil {
		var unavailable *IdentityProviderUnavailableError
		switch {
		case errors.Is(err, ErrLoginRequestInvalid):
			h.writeError(
				writer,
				requestID,
				"login_request_invalid",
				"login request is invalid",
				http.StatusBadRequest,
			)
		case errors.Is(err, ErrLoginNonceInvalid):
			h.writeError(
				writer,
				requestID,
				"login_nonce_invalid",
				"Apple login nonce is invalid or expired",
				http.StatusUnauthorized,
			)
		case errors.Is(err, ErrIdentityTokenInvalid):
			h.writeError(
				writer,
				requestID,
				"identity_token_invalid",
				"Apple identity token is invalid",
				http.StatusUnauthorized,
			)
		case errors.As(err, &unavailable):
			h.writeError(
				writer,
				requestID,
				"identity_provider_unavailable",
				"Apple identity verification is temporarily unavailable",
				http.StatusServiceUnavailable,
			)
		default:
			h.writeError(
				writer,
				requestID,
				"internal_error",
				"internal server error",
				http.StatusInternalServerError,
			)
		}
		return
	}
	h.writeJSON(writer, http.StatusOK, response)
}

func (h *HTTPHandler) handleSamsungPreflight(
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
	var input samsungPreflightRequest
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
	response, err := h.samsung.Preflight(
		request.Context(),
		input.RedirectURI,
	)
	if err != nil {
		if errors.Is(err, ErrLoginRequestInvalid) {
			h.writeError(
				writer,
				requestID,
				"login_request_invalid",
				"login request is invalid",
				http.StatusBadRequest,
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
	h.writeJSON(writer, http.StatusOK, response)
}

func (h *HTTPHandler) handleSamsung(
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
	var input samsungLoginRequest
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
	response, err := h.samsung.Exchange(
		request.Context(),
		input.Code,
		input.State,
		input.Nonce,
		input.CodeVerifier,
		input.RedirectURI,
		input.DeviceID,
	)
	if err != nil {
		var unavailable *IdentityProviderUnavailableError
		switch {
		case errors.Is(err, ErrLoginRequestInvalid):
			h.writeError(
				writer,
				requestID,
				"login_request_invalid",
				"login request is invalid",
				http.StatusBadRequest,
			)
		case errors.Is(err, ErrLoginNonceInvalid):
			h.writeError(
				writer,
				requestID,
				"login_nonce_invalid",
				"Samsung login state is invalid or expired",
				http.StatusUnauthorized,
			)
		case errors.Is(err, ErrIdentityTokenInvalid):
			h.writeError(
				writer,
				requestID,
				"identity_token_invalid",
				"Samsung authorization is invalid",
				http.StatusUnauthorized,
			)
		case errors.As(err, &unavailable):
			h.writeError(
				writer,
				requestID,
				"identity_provider_unavailable",
				"Samsung identity verification is temporarily unavailable",
				http.StatusServiceUnavailable,
			)
		default:
			h.writeError(
				writer,
				requestID,
				"internal_error",
				"internal server error",
				http.StatusInternalServerError,
			)
		}
		return
	}
	h.writeJSON(writer, http.StatusOK, response)
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

func requireEmptyRequestBody(
	writer http.ResponseWriter,
	request *http.Request,
) error {
	request.Body = http.MaxBytesReader(writer, request.Body, 1)
	buffer := make([]byte, 1)
	count, err := request.Body.Read(buffer)
	if count != 0 {
		return errors.New("request body is not empty")
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}
