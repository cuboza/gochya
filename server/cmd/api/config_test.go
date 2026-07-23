package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"strings"
	"testing"
)

func TestLoadAPIConfig(t *testing.T) {
	values := validEnvironment()
	values["GOCHYA_HTTP_ADDRESS"] = "127.0.0.1:9090"
	values["GOCHYA_PLAY_REQUIRED_DEVICE_VERDICTS"] =
		"MEETS_DEVICE_INTEGRITY, MEETS_STRONG_INTEGRITY"
	values["GOCHYA_PLAY_ALLOW_TEST_RESPONSES"] = "true"

	config, err := loadAPIConfig(mapLookup(values))
	if err != nil {
		t.Fatalf("loadAPIConfig: %v", err)
	}
	if config.HTTPAddress != "127.0.0.1:9090" {
		t.Fatalf("HTTPAddress = %q", config.HTTPAddress)
	}
	if len(config.JWTPublicKeys["primary"]) != ed25519.PublicKeySize {
		t.Fatalf("JWT public key length = %d", len(config.JWTPublicKeys["primary"]))
	}
	if len(config.PlayRequiredVerdicts) != 2 ||
		config.PlayRequiredVerdicts[1] != "MEETS_STRONG_INTEGRITY" {
		t.Fatalf("PlayRequiredVerdicts = %#v", config.PlayRequiredVerdicts)
	}
	if !config.PlayAllowTestResponses {
		t.Fatal("PlayAllowTestResponses = false")
	}
	if _, ok := config.AllowedAppBuilds["100"]; !ok {
		t.Fatal("app build allowlist does not contain 100")
	}
}

func TestLoadAPIConfigUsesSecureDefaults(t *testing.T) {
	config, err := loadAPIConfig(mapLookup(validEnvironment()))
	if err != nil {
		t.Fatalf("loadAPIConfig: %v", err)
	}
	if config.HTTPAddress != defaultHTTPAddress {
		t.Fatalf("HTTPAddress = %q", config.HTTPAddress)
	}
	if config.PlayAllowUnlicensed || config.PlayAllowTestResponses {
		t.Fatal("Play Integrity policy defaults are not fail-closed")
	}
	if len(config.PlayRequiredVerdicts) != 1 ||
		config.PlayRequiredVerdicts[0] != "MEETS_DEVICE_INTEGRITY" {
		t.Fatalf("PlayRequiredVerdicts = %#v", config.PlayRequiredVerdicts)
	}
}

func TestLoadAPIConfigRejectsInvalidEnvironment(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]string)
		match  string
	}{
		{
			name: "missing database",
			mutate: func(values map[string]string) {
				delete(values, "GOCHYA_DATABASE_URL")
			},
			match: "GOCHYA_DATABASE_URL is required",
		},
		{
			name: "missing Google client ID",
			mutate: func(values map[string]string) {
				delete(values, "GOCHYA_GOOGLE_CLIENT_IDS")
			},
			match: "GOCHYA_GOOGLE_CLIENT_IDS is required",
		},
		{
			name: "bad address",
			mutate: func(values map[string]string) {
				values["GOCHYA_HTTP_ADDRESS"] = "localhost"
			},
			match: "host:port",
		},
		{
			name: "bad JWT key",
			mutate: func(values map[string]string) {
				values["GOCHYA_JWT_PUBLIC_KEYS_JSON"] = `{"primary":"dG9vLXNob3J0"}`
			},
			match: "32-byte",
		},
		{
			name: "unknown signing key",
			mutate: func(values map[string]string) {
				values["GOCHYA_JWT_SIGNING_KEY_ID"] = "unknown"
			},
			match: "absent",
		},
		{
			name: "mismatched signing key",
			mutate: func(values map[string]string) {
				another := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
				values["GOCHYA_JWT_SIGNING_PRIVATE_KEY"] =
					base64.RawURLEncoding.EncodeToString(another)
			},
			match: "does not match",
		},
		{
			name: "empty CSV item",
			mutate: func(values map[string]string) {
				values["GOCHYA_ALLOWED_APP_BUILDS"] = "100,,101"
			},
			match: "empty item",
		},
		{
			name: "bad boolean",
			mutate: func(values map[string]string) {
				values["GOCHYA_PLAY_ALLOW_UNLICENSED"] = "sometimes"
			},
			match: "must be true or false",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values := validEnvironment()
			test.mutate(values)
			_, err := loadAPIConfig(mapLookup(values))
			if err == nil || !strings.Contains(err.Error(), test.match) {
				t.Fatalf("error = %v, want containing %q", err, test.match)
			}
		})
	}
}

func validEnvironment() map[string]string {
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index)
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	return map[string]string{
		"GOCHYA_DATABASE_URL":                    "postgres://gochya@db/gochya",
		"GOCHYA_JWT_ISSUER":                      "https://auth.gochya.example",
		"GOCHYA_JWT_AUDIENCE":                    "gochya-api",
		"GOCHYA_JWT_PUBLIC_KEYS_JSON":            `{"primary":"` + base64.RawURLEncoding.EncodeToString(publicKey) + `"}`,
		"GOCHYA_JWT_SIGNING_KEY_ID":              "primary",
		"GOCHYA_JWT_SIGNING_PRIVATE_KEY":         base64.RawURLEncoding.EncodeToString(privateKey),
		"GOCHYA_GOOGLE_CLIENT_IDS":               "android-client.apps.googleusercontent.com",
		"GOCHYA_PLAY_PACKAGE_NAME":               "com.gochya.watch",
		"GOCHYA_PLAY_CERTIFICATE_SHA256_DIGESTS": "certificate-a,certificate-b",
		"GOCHYA_ALLOWED_APP_BUILDS":              "100,101",
		"GOCHYA_ALLOWED_CLASSIFIER_VERSIONS":     "punch-v1",
	}
}

func mapLookup(values map[string]string) environmentLookup {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}
