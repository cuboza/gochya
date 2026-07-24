package device

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gochya/gochya/server/internal/dojo"
)

const (
	testPlayer   = "11111111-1111-4111-8111-111111111111"
	testDeviceID = "wear-device-1"
	testAppBuild = "100"
)

func TestServiceRegistersProofBoundAttestedDevice(t *testing.T) {
	now := time.Date(2026, time.July, 23, 10, 0, 0, 0, time.UTC)
	store := newTestStore()
	var verified dojo.AttestationInput
	service := newTestService(t, store, func(_ context.Context, input dojo.AttestationInput) error {
		verified = input
		return nil
	}, now)
	privateKey := testPrivateKey(1)

	preflight, err := service.Preflight(context.Background(), testPlayer, PreflightRequest{
		DeviceID: testDeviceID,
		Platform: PlatformWearOS,
		AppBuild: testAppBuild,
	})
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	if preflight.ExpiresAt != now.Add(defaultChallengeTTL) {
		t.Fatalf("expiresAt = %v", preflight.ExpiresAt)
	}
	request := signedRegisterRequest(t, privateKey, preflight.Challenge)
	response, err := service.Register(context.Background(), testPlayer, request)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if response.DeviceID != testDeviceID ||
		response.Platform != PlatformWearOS ||
		!response.RegisteredAt.Equal(now) {
		t.Fatalf("response = %#v", response)
	}

	canonical, err := CanonicalRegistrationPayload(request)
	if err != nil {
		t.Fatalf("CanonicalRegistrationPayload: %v", err)
	}
	if verified.PlayerID != testPlayer ||
		verified.DeviceID != testDeviceID ||
		verified.AppBuild != testAppBuild ||
		verified.Challenge != preflight.Challenge ||
		verified.Nonce != preflight.Challenge ||
		verified.RequestHash != PlayIntegrityRequestHash(preflight.Challenge, canonical) ||
		verified.Evidence != request.Attestation {
		t.Fatalf("attestation input = %#v", verified)
	}
	device := store.devices[testPlayer+"\x00"+testDeviceID]
	if !bytes.Equal(device.PublicKey, privateKey.Public().(ed25519.PublicKey)) ||
		!device.Enabled {
		t.Fatalf("stored device = %#v", device)
	}
}

func TestRegisterRejectsInvalidProofBeforeAttestation(t *testing.T) {
	store := newTestStore()
	attestationCalls := 0
	service := newTestService(t, store, func(context.Context, dojo.AttestationInput) error {
		attestationCalls++
		return nil
	}, time.Now().UTC())
	preflight, err := service.Preflight(context.Background(), testPlayer, validPreflightRequest())
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	request := signedRegisterRequest(t, testPrivateKey(2), preflight.Challenge)
	signature, err := base64.RawURLEncoding.DecodeString(request.ProofSignature)
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	signature[0] ^= 0xff
	request.ProofSignature = base64.RawURLEncoding.EncodeToString(signature)

	_, err = service.Register(context.Background(), testPlayer, request)
	assertErrorCode(t, err, "signature_invalid")
	if attestationCalls != 0 {
		t.Fatalf("attestation calls = %d", attestationCalls)
	}
	if store.challenges[preflight.Challenge].UsedAt != nil {
		t.Fatal("invalid proof consumed challenge")
	}
}

func TestRegisterMapsAttestationFailuresWithoutConsumingChallenge(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code string
	}{
		{name: "invalid", err: errors.New("bad verdict"), code: "attestation_invalid"},
		{
			name: "unavailable",
			err:  &dojo.AttestationUnavailableError{Cause: errors.New("timeout")},
			code: "attestation_unavailable",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newTestStore()
			service := newTestService(t, store, func(context.Context, dojo.AttestationInput) error {
				return test.err
			}, time.Now().UTC())
			preflight, err := service.Preflight(
				context.Background(),
				testPlayer,
				validPreflightRequest(),
			)
			if err != nil {
				t.Fatalf("Preflight: %v", err)
			}
			_, err = service.Register(
				context.Background(),
				testPlayer,
				signedRegisterRequest(t, testPrivateKey(3), preflight.Challenge),
			)
			assertErrorCode(t, err, test.code)
			if store.challenges[preflight.Challenge].UsedAt != nil {
				t.Fatal("attestation failure consumed challenge")
			}
		})
	}
}

func TestRegisterRejectsReplayAndKeyReplacement(t *testing.T) {
	now := time.Now().UTC()
	store := newTestStore()
	service := newTestService(
		t,
		store,
		func(context.Context, dojo.AttestationInput) error { return nil },
		now,
	)
	firstKey := testPrivateKey(4)
	preflight, err := service.Preflight(context.Background(), testPlayer, validPreflightRequest())
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	request := signedRegisterRequest(t, firstKey, preflight.Challenge)
	if _, err := service.Register(context.Background(), testPlayer, request); err != nil {
		t.Fatalf("Register: %v", err)
	}
	_, err = service.Register(context.Background(), testPlayer, request)
	assertErrorCode(t, err, "enrollment_replay_detected")

	replacementPreflight, err := service.Preflight(
		context.Background(),
		testPlayer,
		validPreflightRequest(),
	)
	if err != nil {
		t.Fatalf("replacement Preflight: %v", err)
	}
	_, err = service.Register(
		context.Background(),
		testPlayer,
		signedRegisterRequest(t, testPrivateKey(5), replacementPreflight.Challenge),
	)
	assertErrorCode(t, err, "device_key_conflict")
	if store.challenges[replacementPreflight.Challenge].UsedAt != nil {
		t.Fatal("key conflict consumed challenge")
	}
}

func TestRegisterRejectsChallengeBindingMismatchAndExpiry(t *testing.T) {
	now := time.Now().UTC()
	store := newTestStore()
	service := newTestService(
		t,
		store,
		func(context.Context, dojo.AttestationInput) error { return nil },
		now,
	)
	preflight, err := service.Preflight(context.Background(), testPlayer, validPreflightRequest())
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	request := signedRegisterRequest(t, testPrivateKey(6), preflight.Challenge)
	request.DeviceID = "another-device"
	_, err = service.Register(context.Background(), testPlayer, request)
	assertErrorCode(t, err, "enrollment_challenge_invalid")

	record := store.challenges[preflight.Challenge]
	record.ExpiresAt = now
	store.challenges[preflight.Challenge] = record
	request = signedRegisterRequest(t, testPrivateKey(6), preflight.Challenge)
	_, err = service.Register(context.Background(), testPlayer, request)
	assertErrorCode(t, err, "enrollment_challenge_invalid")
}

func TestPreflightRejectsUnsupportedPlatformAndBuild(t *testing.T) {
	service := newTestService(
		t,
		newTestStore(),
		func(context.Context, dojo.AttestationInput) error { return nil },
		time.Now().UTC(),
	)
	request := validPreflightRequest()
	request.Platform = "watch_os"
	_, err := service.Preflight(context.Background(), testPlayer, request)
	assertErrorCode(t, err, "platform_unsupported")

	request = validPreflightRequest()
	request.AppBuild = "101"
	_, err = service.Preflight(context.Background(), testPlayer, request)
	assertErrorCode(t, err, "unsupported_build")
}

func TestCanonicalRegistrationPayload(t *testing.T) {
	request := RegisterRequest{
		DeviceID:  "device",
		Platform:  PlatformWearOS,
		AppBuild:  "100",
		Challenge: "challenge",
		PublicKey: "public-key",
	}
	canonical, err := CanonicalRegistrationPayload(request)
	if err != nil {
		t.Fatalf("CanonicalRegistrationPayload: %v", err)
	}
	expected := registrationSignatureDomain +
		"\x00{\"version\":1,\"deviceId\":\"device\",\"platform\":\"wear_os\"," +
		"\"appBuild\":\"100\",\"challenge\":\"challenge\",\"publicKey\":\"public-key\"}"
	if string(canonical) != expected {
		t.Fatalf("canonical = %q", canonical)
	}
	digest := sha256.Sum256(
		append(
			append([]byte(playIntegrityDomain+"\x00challenge"), 0),
			canonical...,
		),
	)
	if actual := PlayIntegrityRequestHash("challenge", canonical); actual !=
		base64.RawURLEncoding.EncodeToString(digest[:]) {
		t.Fatalf("request hash = %q", actual)
	}
}

func validPreflightRequest() PreflightRequest {
	return PreflightRequest{
		DeviceID: testDeviceID,
		Platform: PlatformWearOS,
		AppBuild: testAppBuild,
	}
}

func newTestService(
	t *testing.T,
	store Store,
	attestation dojo.AttestationVerifierFunc,
	now time.Time,
) *Service {
	t.Helper()
	service, err := NewService(ServiceConfig{
		Store:       store,
		Attestation: attestation,
		AllowedAppBuilds: map[string]struct{}{
			testAppBuild: {},
		},
		Now:    func() time.Time { return now },
		Random: rand.Reader,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}

func signedRegisterRequest(
	t *testing.T,
	privateKey ed25519.PrivateKey,
	challenge string,
) RegisterRequest {
	t.Helper()
	request := RegisterRequest{
		DeviceID:  testDeviceID,
		Platform:  PlatformWearOS,
		AppBuild:  testAppBuild,
		Challenge: challenge,
		PublicKey: base64.RawURLEncoding.EncodeToString(
			privateKey.Public().(ed25519.PublicKey),
		),
		Attestation: dojo.AttestationEvidence{
			Provider: "play_integrity_standard",
			Token:    "encrypted-token",
		},
	}
	canonical, err := CanonicalRegistrationPayload(request)
	if err != nil {
		t.Fatalf("CanonicalRegistrationPayload: %v", err)
	}
	request.ProofSignature = base64.RawURLEncoding.EncodeToString(
		ed25519.Sign(privateKey, canonical),
	)
	return request
}

func testPrivateKey(seedByte byte) ed25519.PrivateKey {
	seed := bytes.Repeat([]byte{seedByte}, ed25519.SeedSize)
	return ed25519.NewKeyFromSeed(seed)
}

func assertErrorCode(t *testing.T, err error, expected string) {
	t.Helper()
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != expected {
		t.Fatalf("error = %v, want code %q", err, expected)
	}
}

type testStore struct {
	mu         sync.Mutex
	challenges map[string]ChallengeRecord
	devices    map[string]RegisteredDevice
}

func newTestStore() *testStore {
	return &testStore{
		challenges: make(map[string]ChallengeRecord),
		devices:    make(map[string]RegisteredDevice),
	}
}

func (s *testStore) PutChallenge(_ context.Context, record ChallengeRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.challenges[record.Value] = record
	return nil
}

func (s *testStore) Challenge(_ context.Context, value string) (ChallengeRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.challenges[value]
	if !ok {
		return ChallengeRecord{}, ErrChallengeNotFound
	}
	return record, nil
}

func (s *testStore) CommitRegistration(
	_ context.Context,
	input RegistrationCommit,
) (RegisteredDevice, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	challenge, ok := s.challenges[input.Challenge]
	if !ok ||
		challenge.PlayerID != input.PlayerID ||
		challenge.DeviceID != input.DeviceID ||
		challenge.Platform != input.Platform ||
		challenge.AppBuild != input.AppBuild ||
		!input.Now.Before(challenge.ExpiresAt) {
		return RegisteredDevice{}, ErrChallengeNotFound
	}
	if challenge.UsedAt != nil {
		return RegisteredDevice{}, ErrChallengeUsed
	}
	key := input.PlayerID + "\x00" + input.DeviceID
	if existing, ok := s.devices[key]; ok {
		if !existing.Enabled ||
			existing.Platform != input.Platform ||
			!bytes.Equal(existing.PublicKey, input.PublicKey) {
			return RegisteredDevice{}, ErrDeviceConflict
		}
		challenge.UsedAt = &input.Now
		s.challenges[input.Challenge] = challenge
		return existing, nil
	}
	device := RegisteredDevice{
		PlayerID:     input.PlayerID,
		DeviceID:     input.DeviceID,
		PublicKey:    bytes.Clone(input.PublicKey),
		Platform:     input.Platform,
		Enabled:      true,
		RegisteredAt: input.Now,
	}
	s.devices[key] = device
	challenge.UsedAt = &input.Now
	s.challenges[input.Challenge] = challenge
	return device, nil
}
