package dojo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPPlayIntegrityDecoderCallsGoogleContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost ||
			request.URL.Path != "/v1/com.gochya.watch:decodeIntegrityToken" {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer service-access-token" {
			t.Errorf("Authorization = %q", request.Header.Get("Authorization"))
		}
		var body struct {
			IntegrityToken string `json:"integrity_token"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body.IntegrityToken != "encrypted-integrity-token" {
			t.Errorf("integrity token = %q", body.IntegrityToken)
		}
		_ = json.NewEncoder(writer).Encode(struct {
			TokenPayloadExternal PlayIntegrityPayload `json:"tokenPayloadExternal"`
		}{
			TokenPayloadExternal: validPlayIntegrityPayload(
				newTestTime(),
			),
		})
	}))
	defer server.Close()

	decoder, err := NewHTTPPlayIntegrityDecoder(HTTPPlayIntegrityDecoderConfig{
		AccessTokens: PlayIntegrityAccessTokenSourceFunc(
			func(context.Context) (string, error) {
				return "service-access-token", nil
			},
		),
		HTTPClient:        server.Client(),
		BaseURL:           server.URL,
		AllowInsecureHTTP: true,
	})
	if err != nil {
		t.Fatalf("NewHTTPPlayIntegrityDecoder: %v", err)
	}
	payload, err := decoder.Decode(
		context.Background(),
		testPlayPackage,
		"encrypted-integrity-token",
	)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if payload.RequestDetails.RequestHash != testPlayHash {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestHTTPPlayIntegrityDecoderClassifiesFailures(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		temporary bool
	}{
		{name: "bad token", status: http.StatusBadRequest, temporary: false},
		{name: "credentials", status: http.StatusUnauthorized, temporary: true},
		{name: "quota", status: http.StatusTooManyRequests, temporary: true},
		{name: "google outage", status: http.StatusServiceUnavailable, temporary: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.status)
			}))
			defer server.Close()
			decoder, err := NewHTTPPlayIntegrityDecoder(HTTPPlayIntegrityDecoderConfig{
				AccessTokens: PlayIntegrityAccessTokenSourceFunc(
					func(context.Context) (string, error) { return "access-token", nil },
				),
				HTTPClient:        server.Client(),
				BaseURL:           server.URL,
				AllowInsecureHTTP: true,
			})
			if err != nil {
				t.Fatalf("NewHTTPPlayIntegrityDecoder: %v", err)
			}
			_, err = decoder.Decode(context.Background(), testPlayPackage, "token")
			var unavailable *AttestationUnavailableError
			if errors.As(err, &unavailable) != test.temporary {
				t.Fatalf("error = %v, temporary = %v", err, test.temporary)
			}
		})
	}
}

func TestHTTPPlayIntegrityDecoderRequiresHTTPS(t *testing.T) {
	_, err := NewHTTPPlayIntegrityDecoder(HTTPPlayIntegrityDecoderConfig{
		AccessTokens: PlayIntegrityAccessTokenSourceFunc(
			func(context.Context) (string, error) { return "token", nil },
		),
		BaseURL: "http://playintegrity.example",
	})
	if err == nil {
		t.Fatal("insecure Play Integrity endpoint was accepted")
	}
}

func newTestTime() time.Time {
	return time.Date(2026, 7, 23, 14, 0, 0, 0, time.UTC)
}
