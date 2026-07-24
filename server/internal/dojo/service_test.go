package dojo

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/gochya/gochya/server/internal/corebridge"
)

const (
	testPlayer     = "player-1"
	testDevice     = "watch-1"
	testAppBuild   = "100"
	testClassifier = "punch-v1"
)

type fakeCore struct {
	mu            sync.Mutex
	validateCalls int
	deriveCalls   int
	lastHeart     corebridge.HeartEvidence
	traceIDs      []string
}

func (core *fakeCore) ValidateHeart(
	ctx context.Context,
	heart corebridge.HeartEvidence,
) (corebridge.HeartVerdict, error) {
	core.mu.Lock()
	defer core.mu.Unlock()
	core.validateCalls++
	core.lastHeart = heart
	if traceID, ok := TraceIDFromContext(ctx); ok {
		core.traceIDs = append(core.traceIDs, traceID)
	}
	passed := heart.Present >= 0.8 &&
		heart.MeanBPM >= heart.BaselineBPM+8 &&
		heart.MeanBPM >= 55 &&
		heart.Confidence >= 0.85
	return corebridge.HeartVerdict{Passed: passed, Reason: 1}, nil
}

func (core *fakeCore) DeriveTechnique(
	ctx context.Context,
	metrics corebridge.Metrics,
	_ corebridge.HeartEvidence,
	_ float32,
) (corebridge.TechniqueStats, error) {
	core.mu.Lock()
	defer core.mu.Unlock()
	core.deriveCalls++
	if traceID, ok := TraceIDFromContext(ctx); ok {
		core.traceIDs = append(core.traceIDs, traceID)
	}
	return corebridge.TechniqueStats{
		TechniqueType: metrics.TechniqueType,
		Rarity:        2,
		BaseDamage:    1.04,
		Speed:         66.666664,
		StaminaCost:   3,
		CritChance:    0.0625,
		Quality:       64,
	}, nil
}

type fixture struct {
	service *Service
	store   *MemoryStore
	core    *fakeCore
	private ed25519.PrivateKey
	now     time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index + 1)
	}
	private := ed25519.NewKeyFromSeed(seed)
	store := NewMemoryStore()
	store.RegisterDevice(Device{
		ID:        testDevice,
		PlayerID:  testPlayer,
		PublicKey: private.Public().(ed25519.PublicKey),
		Enabled:   true,
	})
	store.SetActiveElement(testPlayer, 2)
	core := &fakeCore{}
	result := &fixture{
		store:   store,
		core:    core,
		private: private,
		now:     time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC),
	}
	service, err := NewService(ServiceConfig{
		Store:       store,
		Core:        core,
		Attestation: AttestationVerifierFunc(validTestAttestation),
		AllowedAppBuilds: map[string]struct{}{
			testAppBuild: {},
			"101":        {},
		},
		AllowedClassifierVersions: map[string]struct{}{
			testClassifier: {},
		},
		Now:    func() time.Time { return result.now },
		Random: rand.Reader,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	result.service = service
	return result
}

func validTestAttestation(ctx context.Context, input AttestationInput) error {
	traceID, traced := TraceIDFromContext(ctx)
	if input.Challenge == "" || input.Evidence.Provider != "test-integrity" ||
		input.Evidence.Token != "valid-token" || input.RequestHash == "" ||
		!traced || traceID == "" {
		return errors.New("invalid test attestation")
	}
	return nil
}

func (fixture *fixture) preflight(t *testing.T) PreflightResponse {
	t.Helper()
	response, err := fixture.service.Preflight(context.Background(), testPlayer, PreflightRequest{
		DeviceID: testDevice,
		AppBuild: testAppBuild,
	})
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	return response
}

func (fixture *fixture) request(t *testing.T, preflight PreflightResponse) SubmitRequest {
	t.Helper()
	request := SubmitRequest{
		DeviceID:              testDevice,
		Nonce:                 preflight.Nonce,
		EvidenceSchemaVersion: preflight.EvidenceSchemaVersion,
		RecordedAtMS:          fixture.now.UnixMilli(),
		Metrics: Metrics{
			PeakAccelMPS2:   65,
			ExecTimeSeconds: 0.5,
			Precision:       0.8,
			ComboLen:        3,
			RhythmScore:     0.75,
			TechniqueType:   1,
		},
		HeartEvidence: HeartEvidence{
			Present:           0.9,
			MeanBPM:           90,
			DeltaBPM:          20,
			ContactConfidence: 0.9,
		},
		FeatureSummary: FeatureSummary{
			AccelSampleCount:     600,
			GyroSampleCount:      600,
			HeartSampleCount:     30,
			DurationMS:           6000,
			MonotonicStartMS:     1000,
			MonotonicEndMS:       7000,
			AccelPeakMPS2:        65,
			AccelRMSMPS2:         20,
			GyroPeakRadiansS:     10,
			GyroRMSRadiansS:      3,
			EntropyBits:          3.2,
			ZeroCrossings:        100,
			HeartMeanBPM:         90,
			HeartDeltaBPM:        20,
			HeartPresent:         0.9,
			ContactConfidence:    0.9,
			ClassifierID:         "punch",
			ClassifierType:       1,
			ClassifierConfidence: 0.9,
		},
		ClassifierVersion: testClassifier,
		AppBuild:          testAppBuild,
		Attestation: AttestationEvidence{
			Provider: "test-integrity",
			Token:    "valid-token",
		},
	}
	fixture.sign(t, &request)
	return request
}

func (fixture *fixture) sign(t *testing.T, request *SubmitRequest) {
	t.Helper()
	payload, err := CanonicalPayload(*request)
	if err != nil {
		t.Fatalf("CanonicalPayload: %v", err)
	}
	request.PayloadSignature = base64.RawURLEncoding.EncodeToString(
		ed25519.Sign(fixture.private, payload),
	)
}

func TestPreflightIssuesBoundChallenge(t *testing.T) {
	fixture := newFixture(t)
	first := fixture.preflight(t)
	second := fixture.preflight(t)
	if first.Nonce == second.Nonce ||
		first.Challenge == second.Challenge ||
		first.TraceID == second.TraceID {
		t.Fatal("nonce, challenge and trace ID must be unique")
	}
	if err := validateUUID(first.TraceID); err != nil {
		t.Fatalf("trace ID = %q: %v", first.TraceID, err)
	}
	if first.EvidenceSchemaVersion != EvidenceSchemaV1 {
		t.Fatalf("schema = %d", first.EvidenceSchemaVersion)
	}
	if want := fixture.now.Add(5 * time.Minute); !first.ExpiresAt.Equal(want) {
		t.Fatalf("expiresAt = %s, want %s", first.ExpiresAt, want)
	}
}

func TestSubmitIsIdempotentAndPersistsNoDuplicateCard(t *testing.T) {
	fixture := newFixture(t)
	request := fixture.request(t, fixture.preflight(t))
	key := "00000000-0000-4000-8000-000000000001"

	first, err := fixture.service.Submit(context.Background(), testPlayer, key, request)
	if err != nil {
		t.Fatalf("first Submit: %v", err)
	}
	second, err := fixture.service.Submit(context.Background(), testPlayer, key, request)
	if err != nil {
		t.Fatalf("retry Submit: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("retry changed response:\nfirst: %#v\nsecond: %#v", first, second)
	}
	if fixture.store.CardCount() != 1 {
		t.Fatalf("card count = %d", fixture.store.CardCount())
	}
	if fixture.core.deriveCalls != 1 {
		t.Fatalf("derive calls = %d", fixture.core.deriveCalls)
	}
	if fixture.core.lastHeart.BaselineBPM != 70 {
		t.Fatalf("derived baseline = %d", fixture.core.lastHeart.BaselineBPM)
	}
	if first.Card.OwnerID != testPlayer || first.Card.Element != 2 {
		t.Fatalf("server-authoritative card fields = %#v", first.Card)
	}
	if first.TraceID == "" ||
		len(fixture.core.traceIDs) != 2 ||
		fixture.core.traceIDs[0] != first.TraceID ||
		fixture.core.traceIDs[1] != first.TraceID {
		t.Fatalf(
			"Core trace IDs = %#v, response trace ID = %q",
			fixture.core.traceIDs,
			first.TraceID,
		)
	}
}

func TestConcurrentIdempotentRetriesCommitOneCard(t *testing.T) {
	fixture := newFixture(t)
	request := fixture.request(t, fixture.preflight(t))
	key := "00000000-0000-4000-8000-000000000012"
	responses := make([]SubmitResponse, 8)
	failures := make([]error, 8)
	var group sync.WaitGroup
	for index := range responses {
		group.Add(1)
		go func() {
			defer group.Done()
			responses[index], failures[index] = fixture.service.Submit(
				context.Background(),
				testPlayer,
				key,
				request,
			)
		}()
	}
	group.Wait()
	for index, err := range failures {
		if err != nil {
			t.Fatalf("retry %d: %v", index, err)
		}
		if !reflect.DeepEqual(responses[0], responses[index]) {
			t.Fatalf("retry %d returned another card", index)
		}
	}
	if fixture.store.CardCount() != 1 {
		t.Fatalf("card count = %d", fixture.store.CardCount())
	}
}

func TestTamperedSignedPayloadIsRejected(t *testing.T) {
	fixture := newFixture(t)
	preflight := fixture.preflight(t)
	request := fixture.request(t, preflight)
	request.Metrics.Precision = 0.1

	_, err := fixture.service.Submit(
		context.Background(),
		testPlayer,
		"00000000-0000-4000-8000-000000000002",
		request,
	)
	requireErrorCode(t, err, "signature_invalid")
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v", err)
	}
	if apiErr.TraceID != preflight.TraceID {
		t.Fatalf("error trace ID = %q, want %q", apiErr.TraceID, preflight.TraceID)
	}
	if fixture.store.CardCount() != 0 {
		t.Fatal("tampered payload created a card")
	}
}

func TestExpiredNonceIsRejected(t *testing.T) {
	fixture := newFixture(t)
	preflight := fixture.preflight(t)
	request := fixture.request(t, preflight)
	fixture.now = preflight.ExpiresAt

	_, err := fixture.service.Submit(
		context.Background(),
		testPlayer,
		"00000000-0000-4000-8000-000000000003",
		request,
	)
	requireErrorCode(t, err, "nonce_invalid")
}

func TestNonceCannotMoveBetweenAllowedBuilds(t *testing.T) {
	fixture := newFixture(t)
	request := fixture.request(t, fixture.preflight(t))
	request.AppBuild = "101"
	fixture.sign(t, &request)

	_, err := fixture.service.Submit(
		context.Background(),
		testPlayer,
		"00000000-0000-4000-8000-000000000031",
		request,
	)
	requireErrorCode(t, err, "nonce_invalid")
}

func TestUsedNonceWithNewIdempotencyKeyIsReplay(t *testing.T) {
	fixture := newFixture(t)
	request := fixture.request(t, fixture.preflight(t))
	if _, err := fixture.service.Submit(
		context.Background(),
		testPlayer,
		"00000000-0000-4000-8000-000000000004",
		request,
	); err != nil {
		t.Fatalf("first Submit: %v", err)
	}
	_, err := fixture.service.Submit(
		context.Background(),
		testPlayer,
		"00000000-0000-4000-8000-000000000005",
		request,
	)
	requireErrorCode(t, err, "replay_detected")
}

func TestIdempotencyKeyCannotChangeRequest(t *testing.T) {
	fixture := newFixture(t)
	request := fixture.request(t, fixture.preflight(t))
	key := "00000000-0000-4000-8000-000000000006"
	if _, err := fixture.service.Submit(context.Background(), testPlayer, key, request); err != nil {
		t.Fatalf("first Submit: %v", err)
	}
	request.Attestation.Token = "another-token"
	_, err := fixture.service.Submit(context.Background(), testPlayer, key, request)
	requireErrorCode(t, err, "idempotency_conflict")
}

func TestSubmissionIntervalIsEnforced(t *testing.T) {
	fixture := newFixture(t)
	first := fixture.request(t, fixture.preflight(t))
	if _, err := fixture.service.Submit(
		context.Background(),
		testPlayer,
		"00000000-0000-4000-8000-000000000007",
		first,
	); err != nil {
		t.Fatalf("first Submit: %v", err)
	}
	fixture.now = fixture.now.Add(30 * time.Second)
	second := fixture.request(t, fixture.preflight(t))
	_, err := fixture.service.Submit(
		context.Background(),
		testPlayer,
		"00000000-0000-4000-8000-000000000008",
		second,
	)
	requireErrorCode(t, err, "rate_limited")
	if fixture.store.CardCount() != 1 {
		t.Fatalf("card count = %d", fixture.store.CardCount())
	}
}

func TestLowEntropyIsAcceptedAsSuspect(t *testing.T) {
	fixture := newFixture(t)
	request := fixture.request(t, fixture.preflight(t))
	request.FeatureSummary.EntropyBits = 2
	fixture.sign(t, &request)

	response, err := fixture.service.Submit(
		context.Background(),
		testPlayer,
		"00000000-0000-4000-8000-000000000009",
		request,
	)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if response.EvidenceVerdict != "SUSPECT" {
		t.Fatalf("verdict = %q", response.EvidenceVerdict)
	}
}

func TestImpossibleEntropyIsRejected(t *testing.T) {
	fixture := newFixture(t)
	request := fixture.request(t, fixture.preflight(t))
	request.FeatureSummary.EntropyBits = 4.1
	fixture.sign(t, &request)

	_, err := fixture.service.Submit(
		context.Background(),
		testPlayer,
		"00000000-0000-4000-8000-000000000010",
		request,
	)
	requireErrorCode(t, err, "evidence_invalid")
}

func TestInvalidAttestationAndHeartGateFailClosed(t *testing.T) {
	t.Run("attestation", func(t *testing.T) {
		fixture := newFixture(t)
		request := fixture.request(t, fixture.preflight(t))
		request.Attestation.Token = "invalid-token"
		_, err := fixture.service.Submit(
			context.Background(),
			testPlayer,
			"00000000-0000-4000-8000-000000000013",
			request,
		)
		requireErrorCode(t, err, "attestation_invalid")
		if fixture.core.validateCalls != 0 {
			t.Fatal("core was called after attestation failure")
		}
	})

	t.Run("heart", func(t *testing.T) {
		fixture := newFixture(t)
		request := fixture.request(t, fixture.preflight(t))
		request.HeartEvidence.ContactConfidence = 0.8
		request.FeatureSummary.ContactConfidence = 0.8
		fixture.sign(t, &request)
		_, err := fixture.service.Submit(
			context.Background(),
			testPlayer,
			"00000000-0000-4000-8000-000000000014",
			request,
		)
		requireErrorCode(t, err, "heart_rejected")
		if fixture.core.deriveCalls != 0 {
			t.Fatal("card stats were derived after heart rejection")
		}
	})
}

func TestTemporaryAttestationFailureReturnsServiceUnavailable(t *testing.T) {
	fixture := newFixture(t)
	fixture.service.attestation = AttestationVerifierFunc(
		func(context.Context, AttestationInput) error {
			return &AttestationUnavailableError{Cause: errors.New("temporary outage")}
		},
	)
	request := fixture.request(t, fixture.preflight(t))
	_, err := fixture.service.Submit(
		context.Background(),
		testPlayer,
		"00000000-0000-4000-8000-000000000032",
		request,
	)
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "attestation_unavailable" ||
		apiErr.HTTPStatus != 503 {
		t.Fatalf("error = %v", err)
	}
	if fixture.store.CardCount() != 0 {
		t.Fatal("temporary attestation failure created a card")
	}
}

func TestPlayIntegrityRequestHashBindsChallengeAndPayload(t *testing.T) {
	fixture := newFixture(t)
	preflight := fixture.preflight(t)
	request := fixture.request(t, preflight)
	first, err := PlayIntegrityRequestHash(preflight.Challenge, request)
	if err != nil {
		t.Fatalf("PlayIntegrityRequestHash: %v", err)
	}
	second, err := PlayIntegrityRequestHash(preflight.Challenge, request)
	if err != nil {
		t.Fatalf("PlayIntegrityRequestHash retry: %v", err)
	}
	if first != second {
		t.Fatal("request hash is not deterministic")
	}
	request.Metrics.Precision = 0.7
	tampered, err := PlayIntegrityRequestHash(preflight.Challenge, request)
	if err != nil {
		t.Fatalf("tampered PlayIntegrityRequestHash: %v", err)
	}
	anotherChallenge, err := PlayIntegrityRequestHash("another-challenge", request)
	if err != nil {
		t.Fatalf("challenge PlayIntegrityRequestHash: %v", err)
	}
	if first == tampered || tampered == anotherChallenge || first == anotherChallenge {
		t.Fatal("request hash did not bind both challenge and canonical payload")
	}
}

func TestDailyLimitIsEnforced(t *testing.T) {
	fixture := newFixture(t)
	for index := 0; index < 10; index++ {
		request := fixture.request(t, fixture.preflight(t))
		key := "00000000-0000-4000-8000-" + leftPadHex(index+20)
		if _, err := fixture.service.Submit(context.Background(), testPlayer, key, request); err != nil {
			t.Fatalf("Submit %d: %v", index, err)
		}
		fixture.now = fixture.now.Add(time.Minute)
	}
	request := fixture.request(t, fixture.preflight(t))
	_, err := fixture.service.Submit(
		context.Background(),
		testPlayer,
		"00000000-0000-4000-8000-000000000030",
		request,
	)
	requireErrorCode(t, err, "daily_limit")
	if fixture.store.CardCount() != 10 {
		t.Fatalf("card count = %d", fixture.store.CardCount())
	}
}

func leftPadHex(value int) string {
	const digits = "0123456789abcdef"
	result := make([]byte, 12)
	for index := len(result) - 1; index >= 0; index-- {
		result[index] = digits[value&15]
		value >>= 4
	}
	return string(result)
}

func requireErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want API error %q", err, code)
	}
	if apiErr.Code != code {
		t.Fatalf("error code = %q, want %q", apiErr.Code, code)
	}
}
