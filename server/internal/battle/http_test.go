package battle

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gochya/gochya/server/internal/dojo"
)

func TestHTTPMatchHistory(t *testing.T) {
	store := &fakeStore{history: []MatchSummary{{
		ID:         "22222222-2222-4222-8222-222222222222",
		OpponentID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		Mode:       "casual",
		Outcome:    "win",
		CreatedAt:  time.Unix(1, 0).UTC(),
	}}}
	routes := battleRoutes(t, store)
	request := httptest.NewRequest(http.MethodGet, "/v1/me/matches/history?limit=1", nil)
	request.Header.Set("X-Player-ID", battlePlayer)
	recorder := httptest.NewRecorder()

	routes.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	var response []MatchSummary
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response) != 1 || response[0] != store.history[0] || store.historyLimit != 1 {
		t.Fatalf("response = %#v, limit = %d", response, store.historyLimit)
	}
	if recorder.Header().Get("Cache-Control") != "private, no-store" ||
		recorder.Header().Get("X-Request-ID") == "" {
		t.Fatalf("headers = %#v", recorder.Header())
	}
}

func TestHTTPConfirmsMatch(t *testing.T) {
	confirmedAt := time.Unix(2, 0).UTC()
	store := &fakeStore{confirmation: ConfirmResponse{
		MatchID: "22222222-2222-4222-8222-222222222222",
		Outcome: "win",
		Rewards: []Reward{{Currency: "koins", Amount: casualWinKoins}},
		Card: &dojo.TechniqueCard{
			ID:      "33333333-3333-4333-8333-333333333333",
			OwnerID: battlePlayer,
			Rarity:  2,
		},
		ConfirmedAt: confirmedAt,
	}}
	routes := battleRoutes(t, store)
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/match/22222222-2222-4222-8222-222222222222/confirm",
		nil,
	)
	request.Header.Set("X-Player-ID", battlePlayer)
	recorder := httptest.NewRecorder()

	routes.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	var response ConfirmResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.MatchID != store.confirmation.MatchID ||
		response.Outcome != "win" ||
		len(response.Rewards) != 1 ||
		response.Rewards[0].Amount != casualWinKoins ||
		response.Card == nil ||
		response.Card.ID != store.confirmation.Card.ID ||
		store.confirmCommit.PlayerID != battlePlayer {
		t.Fatalf("response = %#v, commit = %#v", response, store.confirmCommit)
	}
}

func TestHTTPConfirmBoundaries(t *testing.T) {
	routes := battleRoutes(t, &fakeStore{})
	matchPath := "/v1/match/22222222-2222-4222-8222-222222222222/confirm"
	tests := []struct {
		name       string
		method     string
		target     string
		body       string
		auth       bool
		wantStatus int
		wantCode   string
	}{
		{
			name:       "authentication required",
			method:     http.MethodPost,
			target:     matchPath,
			wantStatus: http.StatusUnauthorized,
			wantCode:   "unauthenticated",
		},
		{
			name:       "method rejected",
			method:     http.MethodGet,
			target:     matchPath,
			auth:       true,
			wantStatus: http.StatusMethodNotAllowed,
			wantCode:   "method_not_allowed",
		},
		{
			name:       "body rejected",
			method:     http.MethodPost,
			target:     matchPath,
			body:       "{}",
			auth:       true,
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_body",
		},
		{
			name:       "query rejected",
			method:     http.MethodPost,
			target:     matchPath + "?reward=1000",
			auth:       true,
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_query",
		},
		{
			name:       "unknown subroute rejected",
			method:     http.MethodPost,
			target:     "/v1/match/22222222-2222-4222-8222-222222222222/reward",
			auth:       true,
			wantStatus: http.StatusNotFound,
			wantCode:   "route_not_found",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(
				test.method,
				test.target,
				strings.NewReader(test.body),
			)
			if test.auth {
				request.Header.Set("X-Player-ID", battlePlayer)
			}
			recorder := httptest.NewRecorder()
			routes.ServeHTTP(recorder, request)
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
			}
			var response struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if response.Error.Code != test.wantCode {
				t.Fatalf("code = %q, want %q", response.Error.Code, test.wantCode)
			}
		})
	}
}

func TestHTTPMatchHistoryBoundaries(t *testing.T) {
	routes := battleRoutes(t, &fakeStore{})
	tests := []struct {
		name       string
		method     string
		target     string
		auth       bool
		wantStatus int
		wantCode   string
	}{
		{
			name:       "authentication required",
			method:     http.MethodGet,
			target:     "/v1/me/matches/history",
			wantStatus: http.StatusUnauthorized,
			wantCode:   "unauthenticated",
		},
		{
			name:       "method rejected",
			method:     http.MethodPost,
			target:     "/v1/me/matches/history",
			auth:       true,
			wantStatus: http.StatusMethodNotAllowed,
			wantCode:   "method_not_allowed",
		},
		{
			name:       "unknown query rejected",
			method:     http.MethodGet,
			target:     "/v1/me/matches/history?cursor=x",
			auth:       true,
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_query",
		},
		{
			name:       "duplicate limit rejected",
			method:     http.MethodGet,
			target:     "/v1/me/matches/history?limit=1&limit=2",
			auth:       true,
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_query",
		},
		{
			name:       "invalid limit rejected",
			method:     http.MethodGet,
			target:     "/v1/me/matches/history?limit=101",
			auth:       true,
			wantStatus: http.StatusBadRequest,
			wantCode:   "limit_invalid",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.target, nil)
			if test.auth {
				request.Header.Set("X-Player-ID", battlePlayer)
			}
			recorder := httptest.NewRecorder()
			routes.ServeHTTP(recorder, request)
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
			}
			var response struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if response.Error.Code != test.wantCode {
				t.Fatalf("code = %q, want %q", response.Error.Code, test.wantCode)
			}
		})
	}
}

func battleRoutes(t *testing.T, store Store) http.Handler {
	t.Helper()
	service, err := NewService(ServiceConfig{Store: store, Core: fakeCore{}})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	handler, err := NewHTTPHandler(service, dojo.HeaderAuthenticator{}, nil)
	if err != nil {
		t.Fatalf("NewHTTPHandler: %v", err)
	}
	return handler.Routes()
}
