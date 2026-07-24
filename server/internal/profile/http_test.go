package profile

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gochya/gochya/server/internal/dojo"
)

func TestHTTPReadsProfileAndPetsAndActivates(t *testing.T) {
	profile := PlayerProfile{
		ID:          testPlayerID,
		Username:    "player",
		ActivePetID: testPetID,
	}
	pet := testPet(testPetID, true)
	store := &fakeStore{
		profile:   profile,
		pets:      []Pet{pet},
		pet:       pet,
		activated: pet,
	}
	routes := testRoutes(t, store)

	profileRecorder := serveAuthenticated(
		routes,
		http.MethodGet,
		"/v1/me",
		nil,
	)
	if profileRecorder.Code != http.StatusOK {
		t.Fatalf("profile status = %d, body = %s", profileRecorder.Code, profileRecorder.Body)
	}
	var gotProfile PlayerProfile
	if err := json.Unmarshal(profileRecorder.Body.Bytes(), &gotProfile); err != nil {
		t.Fatalf("decode profile: %v", err)
	}
	if !reflect.DeepEqual(gotProfile, profile) {
		t.Fatalf("profile = %#v", gotProfile)
	}

	petsRecorder := serveAuthenticated(
		routes,
		http.MethodGet,
		"/v1/me/pets",
		nil,
	)
	if petsRecorder.Code != http.StatusOK {
		t.Fatalf("pets status = %d, body = %s", petsRecorder.Code, petsRecorder.Body)
	}
	var gotPets []Pet
	if err := json.Unmarshal(petsRecorder.Body.Bytes(), &gotPets); err != nil {
		t.Fatalf("decode pets: %v", err)
	}
	if !reflect.DeepEqual(gotPets, []Pet{pet}) {
		t.Fatalf("pets = %#v", gotPets)
	}

	petRecorder := serveAuthenticated(
		routes,
		http.MethodGet,
		"/v1/me/pets/"+testPetID,
		nil,
	)
	if petRecorder.Code != http.StatusOK {
		t.Fatalf("pet status = %d, body = %s", petRecorder.Code, petRecorder.Body)
	}

	activateRecorder := serveAuthenticated(
		routes,
		http.MethodPost,
		"/v1/me/pets/"+testPetID+"/activate",
		nil,
	)
	if activateRecorder.Code != http.StatusOK {
		t.Fatalf(
			"activate status = %d, body = %s",
			activateRecorder.Code,
			activateRecorder.Body,
		)
	}
	if store.activate.PetID != testPetID || store.activate.PlayerID != testPlayerID {
		t.Fatalf("activate input = %#v", store.activate)
	}
	for name, recorder := range map[string]*httptest.ResponseRecorder{
		"profile":  profileRecorder,
		"pets":     petsRecorder,
		"pet":      petRecorder,
		"activate": activateRecorder,
	} {
		if recorder.Header().Get("Cache-Control") != "private, no-store" ||
			recorder.Header().Get("X-Request-ID") == "" {
			t.Fatalf("%s headers = %#v", name, recorder.Header())
		}
	}
}

func TestHTTPProfileBoundaries(t *testing.T) {
	routes := testRoutes(t, &fakeStore{})
	tests := []struct {
		name       string
		method     string
		target     string
		body       []byte
		auth       bool
		wantStatus int
		wantCode   string
	}{
		{
			name:       "authentication required",
			method:     http.MethodGet,
			target:     "/v1/me",
			wantStatus: http.StatusUnauthorized,
			wantCode:   "unauthenticated",
		},
		{
			name:       "profile method rejected",
			method:     http.MethodPost,
			target:     "/v1/me",
			auth:       true,
			wantStatus: http.StatusMethodNotAllowed,
			wantCode:   "method_not_allowed",
		},
		{
			name:       "list query rejected",
			method:     http.MethodGet,
			target:     "/v1/me/pets?limit=10",
			auth:       true,
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_query",
		},
		{
			name:       "invalid pet ID",
			method:     http.MethodGet,
			target:     "/v1/me/pets/bad",
			auth:       true,
			wantStatus: http.StatusBadRequest,
			wantCode:   "pet_id_invalid",
		},
		{
			name:       "activation body rejected",
			method:     http.MethodPost,
			target:     "/v1/me/pets/" + testPetID + "/activate",
			body:       []byte(`{}`),
			auth:       true,
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_body",
		},
		{
			name:       "activation method rejected",
			method:     http.MethodGet,
			target:     "/v1/me/pets/" + testPetID + "/activate",
			auth:       true,
			wantStatus: http.StatusMethodNotAllowed,
			wantCode:   "method_not_allowed",
		},
		{
			name:       "extra path rejected",
			method:     http.MethodGet,
			target:     "/v1/me/pets/" + testPetID + "/other",
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
				bytes.NewReader(test.body),
			)
			if test.auth {
				request.Header.Set("X-Player-ID", testPlayerID)
			}
			recorder := httptest.NewRecorder()
			routes.ServeHTTP(recorder, request)
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
			}
			var envelope struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("decode error: %v", err)
			}
			if envelope.Error.Code != test.wantCode {
				t.Fatalf("code = %q, want %q", envelope.Error.Code, test.wantCode)
			}
		})
	}
}

func testRoutes(t *testing.T, store Store) http.Handler {
	t.Helper()
	service := testService(t, store)
	handler, err := NewHTTPHandler(service, dojo.HeaderAuthenticator{}, nil)
	if err != nil {
		t.Fatalf("NewHTTPHandler: %v", err)
	}
	return handler.Routes()
}

func serveAuthenticated(
	handler http.Handler,
	method string,
	target string,
	body []byte,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	request.Header.Set("X-Player-ID", testPlayerID)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}
