package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

const defaultHTTPAddress = ":8080"

type apiConfig struct {
	HTTPAddress             string
	DatabaseURL             string
	JWTIssuer               string
	JWTAudience             string
	JWTPublicKeys           map[string]ed25519.PublicKey
	JWTSigningKeyID         string
	JWTSigningPrivateKey    ed25519.PrivateKey
	GoogleClientIDs         map[string]struct{}
	PlayPackageName         string
	PlayCertificateDigests  map[string]struct{}
	PlayRequiredVerdicts    []string
	PlayAllowUnlicensed     bool
	PlayAllowTestResponses  bool
	AllowedAppBuilds        map[string]struct{}
	AllowedClassifierBuilds map[string]struct{}
}

type environmentLookup func(string) (string, bool)

func loadAPIConfig(lookup environmentLookup) (apiConfig, error) {
	httpAddress := optionalEnvironment(lookup, "GOCHYA_HTTP_ADDRESS", defaultHTTPAddress)
	if err := validateHTTPAddress(httpAddress); err != nil {
		return apiConfig{}, fmt.Errorf("GOCHYA_HTTP_ADDRESS: %w", err)
	}
	databaseURL, err := requiredEnvironment(lookup, "GOCHYA_DATABASE_URL")
	if err != nil {
		return apiConfig{}, err
	}
	jwtIssuer, err := requiredEnvironment(lookup, "GOCHYA_JWT_ISSUER")
	if err != nil {
		return apiConfig{}, err
	}
	jwtAudience, err := requiredEnvironment(lookup, "GOCHYA_JWT_AUDIENCE")
	if err != nil {
		return apiConfig{}, err
	}
	jwtKeysJSON, err := requiredEnvironment(lookup, "GOCHYA_JWT_PUBLIC_KEYS_JSON")
	if err != nil {
		return apiConfig{}, err
	}
	jwtKeys, err := parseJWTKeys(jwtKeysJSON)
	if err != nil {
		return apiConfig{}, fmt.Errorf("GOCHYA_JWT_PUBLIC_KEYS_JSON: %w", err)
	}
	jwtSigningKeyID, err := requiredEnvironment(lookup, "GOCHYA_JWT_SIGNING_KEY_ID")
	if err != nil {
		return apiConfig{}, err
	}
	serializedPrivateKey, err := requiredEnvironment(
		lookup,
		"GOCHYA_JWT_SIGNING_PRIVATE_KEY",
	)
	if err != nil {
		return apiConfig{}, err
	}
	jwtSigningPrivateKey, err := parseJWTPrivateKey(serializedPrivateKey)
	if err != nil {
		return apiConfig{}, fmt.Errorf("GOCHYA_JWT_SIGNING_PRIVATE_KEY: %w", err)
	}
	signingPublicKey, ok := jwtKeys[jwtSigningKeyID]
	if !ok {
		return apiConfig{}, errors.New(
			"GOCHYA_JWT_SIGNING_KEY_ID is absent from GOCHYA_JWT_PUBLIC_KEYS_JSON",
		)
	}
	if !bytes.Equal(signingPublicKey, jwtSigningPrivateKey.Public().(ed25519.PublicKey)) {
		return apiConfig{}, errors.New(
			"JWT signing private key does not match its configured public key",
		)
	}
	googleClientIDs, err := requiredCSVSet(lookup, "GOCHYA_GOOGLE_CLIENT_IDS")
	if err != nil {
		return apiConfig{}, err
	}
	playPackageName, err := requiredEnvironment(lookup, "GOCHYA_PLAY_PACKAGE_NAME")
	if err != nil {
		return apiConfig{}, err
	}
	playCertificates, err := requiredCSVSet(
		lookup,
		"GOCHYA_PLAY_CERTIFICATE_SHA256_DIGESTS",
	)
	if err != nil {
		return apiConfig{}, err
	}
	requiredVerdicts, err := optionalCSVList(
		lookup,
		"GOCHYA_PLAY_REQUIRED_DEVICE_VERDICTS",
		[]string{"MEETS_DEVICE_INTEGRITY"},
	)
	if err != nil {
		return apiConfig{}, err
	}
	allowUnlicensed, err := optionalBool(
		lookup,
		"GOCHYA_PLAY_ALLOW_UNLICENSED",
		false,
	)
	if err != nil {
		return apiConfig{}, err
	}
	allowTestResponses, err := optionalBool(
		lookup,
		"GOCHYA_PLAY_ALLOW_TEST_RESPONSES",
		false,
	)
	if err != nil {
		return apiConfig{}, err
	}
	appBuilds, err := requiredCSVSet(lookup, "GOCHYA_ALLOWED_APP_BUILDS")
	if err != nil {
		return apiConfig{}, err
	}
	classifierBuilds, err := requiredCSVSet(
		lookup,
		"GOCHYA_ALLOWED_CLASSIFIER_VERSIONS",
	)
	if err != nil {
		return apiConfig{}, err
	}
	return apiConfig{
		HTTPAddress:             httpAddress,
		DatabaseURL:             databaseURL,
		JWTIssuer:               jwtIssuer,
		JWTAudience:             jwtAudience,
		JWTPublicKeys:           jwtKeys,
		JWTSigningKeyID:         jwtSigningKeyID,
		JWTSigningPrivateKey:    jwtSigningPrivateKey,
		GoogleClientIDs:         googleClientIDs,
		PlayPackageName:         playPackageName,
		PlayCertificateDigests:  playCertificates,
		PlayRequiredVerdicts:    requiredVerdicts,
		PlayAllowUnlicensed:     allowUnlicensed,
		PlayAllowTestResponses:  allowTestResponses,
		AllowedAppBuilds:        appBuilds,
		AllowedClassifierBuilds: classifierBuilds,
	}, nil
}

func requiredEnvironment(lookup environmentLookup, name string) (string, error) {
	value, ok := lookup(name)
	value = strings.TrimSpace(value)
	if !ok || value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func optionalEnvironment(
	lookup environmentLookup,
	name string,
	fallback string,
) string {
	value, ok := lookup(name)
	value = strings.TrimSpace(value)
	if !ok || value == "" {
		return fallback
	}
	return value
}

func requiredCSVSet(
	lookup environmentLookup,
	name string,
) (map[string]struct{}, error) {
	value, err := requiredEnvironment(lookup, name)
	if err != nil {
		return nil, err
	}
	values, err := parseCSVList(value)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	result := make(map[string]struct{}, len(values))
	for _, item := range values {
		result[item] = struct{}{}
	}
	return result, nil
}

func optionalCSVList(
	lookup environmentLookup,
	name string,
	fallback []string,
) ([]string, error) {
	value, ok := lookup(name)
	if !ok || strings.TrimSpace(value) == "" {
		return append([]string(nil), fallback...), nil
	}
	result, err := parseCSVList(value)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return result, nil
}

func parseCSVList(value string) ([]string, error) {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			return nil, errors.New("list contains an empty item")
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	if len(result) == 0 {
		return nil, errors.New("list must not be empty")
	}
	return result, nil
}

func optionalBool(
	lookup environmentLookup,
	name string,
	fallback bool,
) (bool, error) {
	value, ok := lookup(name)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	result, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", name)
	}
	return result, nil
}

func parseJWTKeys(value string) (map[string]ed25519.PublicKey, error) {
	var encoded map[string]string
	if err := json.Unmarshal([]byte(value), &encoded); err != nil {
		return nil, fmt.Errorf("decode key map: %w", err)
	}
	if len(encoded) == 0 {
		return nil, errors.New("key map must not be empty")
	}
	result := make(map[string]ed25519.PublicKey, len(encoded))
	for keyID, serialized := range encoded {
		if strings.TrimSpace(keyID) == "" {
			return nil, errors.New("key ID must not be empty")
		}
		publicKey, err := base64.RawURLEncoding.DecodeString(serialized)
		if err != nil || len(publicKey) != ed25519.PublicKeySize {
			return nil, fmt.Errorf(
				"key %q must be a %d-byte unpadded base64url Ed25519 public key",
				keyID,
				ed25519.PublicKeySize,
			)
		}
		result[keyID] = ed25519.PublicKey(publicKey)
	}
	return result, nil
}

func parseJWTPrivateKey(value string) (ed25519.PrivateKey, error) {
	privateKey, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(privateKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf(
			"must be a %d-byte unpadded base64url Ed25519 private key",
			ed25519.PrivateKeySize,
		)
	}
	return ed25519.PrivateKey(privateKey), nil
}

func validateHTTPAddress(address string) error {
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return errors.New("must use host:port syntax")
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	return nil
}
