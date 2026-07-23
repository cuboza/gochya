package dojo

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"
)

const (
	testPlayPackage = "com.gochya.watch"
	testPlayCert    = "base64url-certificate-digest"
	testPlayHash    = "base64url-request-hash"
)

type fakePlayIntegrityDecoder struct {
	payload     PlayIntegrityPayload
	err         error
	packageName string
	token       string
}

func (decoder *fakePlayIntegrityDecoder) Decode(
	_ context.Context,
	packageName string,
	token string,
) (PlayIntegrityPayload, error) {
	decoder.packageName = packageName
	decoder.token = token
	return decoder.payload, decoder.err
}

func TestPlayIntegrityVerifierAcceptsBoundRecognizedDevice(t *testing.T) {
	now := time.Date(2026, 7, 23, 14, 0, 0, 0, time.UTC)
	decoder := &fakePlayIntegrityDecoder{payload: validPlayIntegrityPayload(now)}
	verifier := newTestPlayIntegrityVerifier(t, decoder, now)
	if err := verifier.Verify(context.Background(), validPlayIntegrityInput()); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if decoder.packageName != testPlayPackage || decoder.token != "encrypted-token" {
		t.Fatalf("decoder input = %q, %q", decoder.packageName, decoder.token)
	}
}

func TestPlayIntegrityVerifierRejectsBadVerdicts(t *testing.T) {
	now := time.Date(2026, 7, 23, 14, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		mutate func(*PlayIntegrityPayload, *AttestationInput)
	}{
		{
			name: "wrong provider",
			mutate: func(_ *PlayIntegrityPayload, input *AttestationInput) {
				input.Evidence.Provider = "classic"
			},
		},
		{
			name: "request hash mismatch",
			mutate: func(payload *PlayIntegrityPayload, _ *AttestationInput) {
				payload.RequestDetails.RequestHash = "another-hash"
			},
		},
		{
			name: "stale verdict",
			mutate: func(payload *PlayIntegrityPayload, _ *AttestationInput) {
				payload.RequestDetails.TimestampMillis = strconv.FormatInt(
					now.Add(-3*time.Minute).UnixMilli(),
					10,
				)
			},
		},
		{
			name: "unrecognized app",
			mutate: func(payload *PlayIntegrityPayload, _ *AttestationInput) {
				payload.AppIntegrity.AppRecognitionVerdict = "UNRECOGNIZED_VERSION"
			},
		},
		{
			name: "wrong version",
			mutate: func(payload *PlayIntegrityPayload, _ *AttestationInput) {
				payload.AppIntegrity.VersionCode = "101"
			},
		},
		{
			name: "wrong certificate",
			mutate: func(payload *PlayIntegrityPayload, _ *AttestationInput) {
				payload.AppIntegrity.CertificateSHA256Digest = []string{"another-certificate"}
			},
		},
		{
			name: "unlicensed",
			mutate: func(payload *PlayIntegrityPayload, _ *AttestationInput) {
				payload.AccountDetails.AppLicensingVerdict = "UNLICENSED"
			},
		},
		{
			name: "device compromised",
			mutate: func(payload *PlayIntegrityPayload, _ *AttestationInput) {
				payload.DeviceIntegrity.DeviceRecognitionVerdict = nil
			},
		},
		{
			name: "testing response",
			mutate: func(payload *PlayIntegrityPayload, _ *AttestationInput) {
				payload.TestingDetails.IsTestingResponse = true
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload := validPlayIntegrityPayload(now)
			input := validPlayIntegrityInput()
			test.mutate(&payload, &input)
			verifier := newTestPlayIntegrityVerifier(
				t,
				&fakePlayIntegrityDecoder{payload: payload},
				now,
			)
			if err := verifier.Verify(context.Background(), input); err == nil {
				t.Fatal("invalid verdict was accepted")
			}
		})
	}
}

func TestPlayIntegrityVerifierPreservesTemporaryDecoderError(t *testing.T) {
	now := time.Date(2026, 7, 23, 14, 0, 0, 0, time.UTC)
	unavailable := &AttestationUnavailableError{Cause: errors.New("Google timeout")}
	decoder := &countingPlayIntegrityDecoder{err: unavailable}
	verifier := newTestPlayIntegrityVerifier(
		t,
		decoder,
		now,
	)
	for attempt := 0; attempt < 2; attempt++ {
		err := verifier.Verify(context.Background(), validPlayIntegrityInput())
		var target *AttestationUnavailableError
		if !errors.As(err, &target) {
			t.Fatalf("attempt %d error = %v", attempt, err)
		}
	}
	if calls := decoder.Calls(); calls != 2 {
		t.Fatalf("decoder calls = %d, want 2", calls)
	}
}

func TestPlayIntegrityVerifierCoalescesAndCachesDecode(t *testing.T) {
	now := time.Date(2026, 7, 23, 14, 0, 0, 0, time.UTC)
	decoder := &blockingPlayIntegrityDecoder{
		payload: validPlayIntegrityPayload(now),
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	verifier := newTestPlayIntegrityVerifier(t, decoder, now)

	const requests = 8
	errorsByRequest := make(chan error, requests)
	for request := 0; request < requests; request++ {
		go func() {
			errorsByRequest <- verifier.Verify(
				context.Background(),
				validPlayIntegrityInput(),
			)
		}()
	}
	select {
	case <-decoder.started:
	case <-time.After(time.Second):
		t.Fatal("decoder did not start")
	}
	close(decoder.release)
	for request := 0; request < requests; request++ {
		if err := <-errorsByRequest; err != nil {
			t.Fatalf("Verify: %v", err)
		}
	}
	if calls := decoder.Calls(); calls != 1 {
		t.Fatalf("concurrent decoder calls = %d, want 1", calls)
	}
	if err := verifier.Verify(
		context.Background(),
		validPlayIntegrityInput(),
	); err != nil {
		t.Fatalf("cached Verify: %v", err)
	}
	if calls := decoder.Calls(); calls != 1 {
		t.Fatalf("cached decoder calls = %d, want 1", calls)
	}
}

type countingPlayIntegrityDecoder struct {
	mu      sync.Mutex
	payload PlayIntegrityPayload
	err     error
	calls   int
}

func (decoder *countingPlayIntegrityDecoder) Decode(
	_ context.Context,
	_ string,
	_ string,
) (PlayIntegrityPayload, error) {
	decoder.mu.Lock()
	defer decoder.mu.Unlock()
	decoder.calls++
	return decoder.payload, decoder.err
}

func (decoder *countingPlayIntegrityDecoder) Calls() int {
	decoder.mu.Lock()
	defer decoder.mu.Unlock()
	return decoder.calls
}

type blockingPlayIntegrityDecoder struct {
	mu          sync.Mutex
	payload     PlayIntegrityPayload
	started     chan struct{}
	release     chan struct{}
	startedOnce sync.Once
	calls       int
}

func (decoder *blockingPlayIntegrityDecoder) Decode(
	ctx context.Context,
	_ string,
	_ string,
) (PlayIntegrityPayload, error) {
	decoder.mu.Lock()
	decoder.calls++
	decoder.mu.Unlock()
	decoder.startedOnce.Do(func() { close(decoder.started) })
	select {
	case <-decoder.release:
		return decoder.payload, nil
	case <-ctx.Done():
		return PlayIntegrityPayload{}, ctx.Err()
	}
}

func (decoder *blockingPlayIntegrityDecoder) Calls() int {
	decoder.mu.Lock()
	defer decoder.mu.Unlock()
	return decoder.calls
}

func newTestPlayIntegrityVerifier(
	t *testing.T,
	decoder PlayIntegrityDecoder,
	now time.Time,
) *PlayIntegrityVerifier {
	t.Helper()
	verifier, err := NewPlayIntegrityVerifier(PlayIntegrityVerifierConfig{
		Decoder:     decoder,
		PackageName: testPlayPackage,
		CertificateSHA256Digests: map[string]struct{}{
			testPlayCert: {},
		},
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewPlayIntegrityVerifier: %v", err)
	}
	return verifier
}

func validPlayIntegrityInput() AttestationInput {
	return AttestationInput{
		AppBuild:    testAppBuild,
		RequestHash: testPlayHash,
		Evidence: AttestationEvidence{
			Provider: playIntegrityStandardProvider,
			Token:    "encrypted-token",
		},
	}
}

func validPlayIntegrityPayload(now time.Time) PlayIntegrityPayload {
	var payload PlayIntegrityPayload
	payload.RequestDetails.RequestPackageName = testPlayPackage
	payload.RequestDetails.RequestHash = testPlayHash
	payload.RequestDetails.TimestampMillis = strconv.FormatInt(now.UnixMilli(), 10)
	payload.AppIntegrity.AppRecognitionVerdict = "PLAY_RECOGNIZED"
	payload.AppIntegrity.PackageName = testPlayPackage
	payload.AppIntegrity.CertificateSHA256Digest = []string{testPlayCert}
	payload.AppIntegrity.VersionCode = testAppBuild
	payload.AccountDetails.AppLicensingVerdict = "LICENSED"
	payload.DeviceIntegrity.DeviceRecognitionVerdict = []string{"MEETS_DEVICE_INTEGRITY"}
	return payload
}
