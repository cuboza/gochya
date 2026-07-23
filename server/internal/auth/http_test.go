package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeSessionManager struct {
	pair        TokenPair
	refreshErr  error
	logoutErr   error
	refreshSeen string
	logoutSeen  string
}

type fakeGoogleExchanger struct {
	response LoginResponse
	err      error
	token    string
	deviceID string
}

func (exchange *fakeGoogleExchanger) Exchange(
	_ context.Context,
	token string,
	deviceID string,
) (LoginResponse, error) {
	exchange.token = token
	exchange.deviceID = deviceID
	return exchange.response, exchange.err
}

func TestHTTPGoogleReturnsSessionAndPlayer(t *testing.T) {
	google := &fakeGoogleExchanger{response: LoginResponse{
		TokenPair: TokenPair{JWT: "access", RefreshToken: "refresh"},
		Player: Player{
			ID:       "77777777-7777-4777-8777-777777777777",
			Username: "google_player",
		},
	}}
	handler, err := NewHTTPHandlerWithGoogle(
		&fakeSessionManager{},
		google,
		nil,
	)
	if err != nil {
		t.Fatalf("NewHTTPHandlerWithGoogle: %v", err)
	}
	response := httptest.NewRecorder()
	handler.Routes().ServeHTTP(
		response,
		httptest.NewRequest(
			http.MethodPost,
			"/v1/auth/google",
			strings.NewReader(
				`{"idToken":"google-token","deviceId":"phone-1"}`,
			),
		),
	)
	if response.Code != http.StatusOK ||
		google.token != "google-token" ||
		google.deviceID != "phone-1" ||
		!strings.Contains(response.Body.String(), `"username":"google_player"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestHTTPGoogleMapsIdentityFailures(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{
			name:   "invalid token",
			err:    ErrIdentityTokenInvalid,
			status: http.StatusUnauthorized,
			code:   "identity_token_invalid",
		},
		{
			name: "provider unavailable",
			err: &IdentityProviderUnavailableError{
				Cause: errors.New("timeout"),
			},
			status: http.StatusServiceUnavailable,
			code:   "identity_provider_unavailable",
		},
		{
			name:   "invalid request",
			err:    ErrLoginRequestInvalid,
			status: http.StatusBadRequest,
			code:   "login_request_invalid",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler, err := NewHTTPHandlerWithGoogle(
				&fakeSessionManager{},
				&fakeGoogleExchanger{err: test.err},
				nil,
			)
			if err != nil {
				t.Fatalf("NewHTTPHandlerWithGoogle: %v", err)
			}
			response := httptest.NewRecorder()
			handler.Routes().ServeHTTP(
				response,
				httptest.NewRequest(
					http.MethodPost,
					"/v1/auth/google",
					strings.NewReader(`{"idToken":"token"}`),
				),
			)
			if response.Code != test.status ||
				!strings.Contains(
					response.Body.String(),
					`"code":"`+test.code+`"`,
				) {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func (manager *fakeSessionManager) Refresh(
	_ context.Context,
	token string,
) (TokenPair, error) {
	manager.refreshSeen = token
	return manager.pair, manager.refreshErr
}

func (manager *fakeSessionManager) Logout(
	_ context.Context,
	token string,
) error {
	manager.logoutSeen = token
	return manager.logoutErr
}

func TestHTTPRefreshReturnsRotatedPair(t *testing.T) {
	manager := &fakeSessionManager{pair: TokenPair{
		JWT:                   "access-token",
		RefreshToken:          "replacement-token",
		AccessTokenExpiresAt:  time.Date(2026, 7, 23, 12, 15, 0, 0, time.UTC),
		RefreshTokenExpiresAt: time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC),
	}}
	handler := newTestHTTPHandler(t, manager)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/auth/refresh",
		strings.NewReader(`{"refreshToken":"current-token"}`),
	)
	handler.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK ||
		!strings.Contains(response.Body.String(), `"jwt":"access-token"`) ||
		manager.refreshSeen != "current-token" {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" ||
		response.Header().Get("X-Request-ID") == "" {
		t.Fatalf("response headers = %#v", response.Header())
	}
}

func TestHTTPRefreshHidesReuseDetails(t *testing.T) {
	manager := &fakeSessionManager{refreshErr: ErrRefreshTokenReused}
	handler := newTestHTTPHandler(t, manager)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/auth/refresh",
		strings.NewReader(`{"refreshToken":"stolen-token"}`),
	)
	handler.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized ||
		!strings.Contains(response.Body.String(), `"code":"refresh_token_invalid"`) ||
		strings.Contains(response.Body.String(), "reuse") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestHTTPLogoutIsIdempotent(t *testing.T) {
	manager := &fakeSessionManager{}
	handler := newTestHTTPHandler(t, manager)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/auth/logout",
		strings.NewReader(`{"refreshToken":"current-token"}`),
	)
	handler.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusNoContent || manager.logoutSeen != "current-token" {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestHTTPAuthRejectsUnknownFieldsAndMethods(t *testing.T) {
	handler := newTestHTTPHandler(t, &fakeSessionManager{})
	unknown := httptest.NewRecorder()
	handler.Routes().ServeHTTP(
		unknown,
		httptest.NewRequest(
			http.MethodPost,
			"/v1/auth/refresh",
			strings.NewReader(`{"refreshToken":"token","rawCredential":"secret"}`),
		),
	)
	if unknown.Code != http.StatusBadRequest {
		t.Fatalf("unknown-field status = %d", unknown.Code)
	}

	method := httptest.NewRecorder()
	handler.Routes().ServeHTTP(
		method,
		httptest.NewRequest(http.MethodGet, "/v1/auth/logout", nil),
	)
	if method.Code != http.StatusMethodNotAllowed ||
		method.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("method response = %d, Allow = %q", method.Code, method.Header().Get("Allow"))
	}
}

func TestHTTPLogoutMapsStoreFailure(t *testing.T) {
	manager := &fakeSessionManager{logoutErr: errors.New("database unavailable")}
	handler := newTestHTTPHandler(t, manager)
	response := httptest.NewRecorder()
	handler.Routes().ServeHTTP(
		response,
		httptest.NewRequest(
			http.MethodPost,
			"/v1/auth/logout",
			strings.NewReader(`{"refreshToken":"token"}`),
		),
	)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", response.Code)
	}
}

func newTestHTTPHandler(
	t *testing.T,
	manager SessionManager,
) *HTTPHandler {
	t.Helper()
	handler, err := NewHTTPHandler(manager, nil)
	if err != nil {
		t.Fatalf("NewHTTPHandler: %v", err)
	}
	return handler
}
