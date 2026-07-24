//go:build cgo && gochya_core

package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gochya/gochya/server/internal/corebridge"
	"github.com/gochya/gochya/server/internal/dojo"
	"github.com/gochya/gochya/server/internal/inventory"
	"github.com/jackc/pgx/v5/pgxpool"
)

const verticalSlicePlayer = "11111111-1111-4111-8111-111111111111"

func TestDojoCardAppearsInTechniqueInventory(t *testing.T) {
	databaseURL := os.Getenv("GOCHYA_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("GOCHYA_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool := schemaValidationTestPool(t, ctx, databaseURL)
	applyAllUpMigrations(t, ctx, pool)

	privateKey := verticalSliceSeed(t, ctx, pool)
	dojoStore, err := dojo.NewPostgresStore(dojo.PostgresStoreConfig{Pool: pool})
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	dojoService, err := dojo.NewService(dojo.ServiceConfig{
		Store: dojoStore,
		Core:  corebridge.NativeEngine{},
		Attestation: dojo.AttestationVerifierFunc(
			func(ctx context.Context, input dojo.AttestationInput) error {
				if traceID, ok := dojo.TraceIDFromContext(ctx); !ok || traceID == "" {
					t.Fatal("attestation did not receive trace ID")
				}
				if input.RequestHash == "" {
					t.Fatal("attestation did not receive request hash")
				}
				return nil
			},
		),
		AllowedAppBuilds: map[string]struct{}{
			"100": {},
		},
		AllowedClassifierVersions: map[string]struct{}{
			"punch-v1": {},
		},
		Now:    func() time.Time { return now },
		Random: rand.Reader,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	preflight, err := dojoService.Preflight(ctx, verticalSlicePlayer, dojo.PreflightRequest{
		DeviceID: "watch-1",
		AppBuild: "100",
	})
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	request := verticalSliceSubmitRequest(t, privateKey, preflight, now)
	idempotencyKey := "55555555-5555-4555-8555-555555555555"
	submitted, err := dojoService.Submit(
		ctx,
		verticalSlicePlayer,
		idempotencyKey,
		request,
	)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	retried, err := dojoService.Submit(
		ctx,
		verticalSlicePlayer,
		idempotencyKey,
		request,
	)
	if err != nil {
		t.Fatalf("idempotent Submit: %v", err)
	}
	if !reflect.DeepEqual(submitted, retried) {
		t.Fatalf("idempotent response changed: %#v != %#v", submitted, retried)
	}

	inventoryStore, err := inventory.NewPostgresStore(pool)
	if err != nil {
		t.Fatalf("NewPostgresStore inventory: %v", err)
	}
	inventoryService, err := inventory.NewService(inventoryStore)
	if err != nil {
		t.Fatalf("NewService inventory: %v", err)
	}
	page, err := inventoryService.ListTechniques(
		ctx,
		verticalSlicePlayer,
		"20",
		"",
	)
	if err != nil {
		t.Fatalf("ListTechniques: %v", err)
	}
	if len(page.Items) != 1 || !reflect.DeepEqual(page.Items[0], submitted.Card) {
		t.Fatalf("inventory page = %#v, submitted card = %#v", page, submitted.Card)
	}
	encodedPage, err := json.Marshal(page)
	if err != nil {
		t.Fatalf("marshal inventory page: %v", err)
	}
	for _, forbidden := range []string{
		"attestation",
		"heartEvidence",
		"featureSummary",
		"payloadSignature",
	} {
		if strings.Contains(string(encodedPage), forbidden) {
			t.Fatalf("inventory exposed %q: %s", forbidden, encodedPage)
		}
	}

	var auditTraceID string
	if err := pool.QueryRow(
		ctx,
		`SELECT trace_id::text
		   FROM dojo_submission_audit
		  WHERE card_id = $1`,
		submitted.Card.ID,
	).Scan(&auditTraceID); err != nil {
		t.Fatalf("query audit trace ID: %v", err)
	}
	if submitted.TraceID != preflight.TraceID || auditTraceID != preflight.TraceID {
		t.Fatalf(
			"trace IDs: preflight=%q submit=%q audit=%q",
			preflight.TraceID,
			submitted.TraceID,
			auditTraceID,
		)
	}
}

func verticalSliceSeed(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
) ed25519.PrivateKey {
	t.Helper()
	if _, err := pool.Exec(
		ctx,
		`INSERT INTO players (
		     id, username, created_at, auth_method, auth_subject
		 ) VALUES ($1, 'vertical-player', NOW(), 'google', 'vertical-subject')`,
		verticalSlicePlayer,
	); err != nil {
		t.Fatalf("seed player: %v", err)
	}
	if _, err := pool.Exec(
		ctx,
		`INSERT INTO pets (
		     id, owner_id, genome, stage, needs, stats, is_active, created_at
		 ) VALUES (
		     '22222222-2222-4222-8222-222222222222',
		     $1,
		     '{"element":"Earth"}',
		     'baby',
		     '{"hunger":80,"energy":70,"hygiene":60,"mood":90}',
		     '{"str":1,"agi":2,"end":3,"foc":4}',
		     TRUE,
		     NOW()
		 )`,
		verticalSlicePlayer,
	); err != nil {
		t.Fatalf("seed active pet: %v", err)
	}
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index + 1)
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	if _, err := pool.Exec(
		ctx,
		`INSERT INTO registered_devices (
		     player_id, device_id, public_key, platform, enabled
		 ) VALUES ($1, 'watch-1', $2, 'wear_os', TRUE)`,
		verticalSlicePlayer,
		privateKey.Public().(ed25519.PublicKey),
	); err != nil {
		t.Fatalf("seed registered device: %v", err)
	}
	return privateKey
}

func verticalSliceSubmitRequest(
	t *testing.T,
	privateKey ed25519.PrivateKey,
	preflight dojo.PreflightResponse,
	now time.Time,
) dojo.SubmitRequest {
	t.Helper()
	request := dojo.SubmitRequest{
		DeviceID:              "watch-1",
		Nonce:                 preflight.Nonce,
		EvidenceSchemaVersion: preflight.EvidenceSchemaVersion,
		RecordedAtMS:          now.UnixMilli(),
		Metrics: dojo.Metrics{
			PeakAccelMPS2:   65,
			ExecTimeSeconds: 0.5,
			Precision:       0.8,
			ComboLen:        3,
			RhythmScore:     0.75,
			TechniqueType:   1,
		},
		HeartEvidence: dojo.HeartEvidence{
			Present:           0.9,
			MeanBPM:           90,
			DeltaBPM:          20,
			ContactConfidence: 0.9,
		},
		FeatureSummary: dojo.FeatureSummary{
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
		ClassifierVersion: "punch-v1",
		AppBuild:          "100",
		Attestation: dojo.AttestationEvidence{
			Provider: "integration",
			Token:    "integration-token",
		},
	}
	canonical, err := dojo.CanonicalPayload(request)
	if err != nil {
		t.Fatalf("CanonicalPayload: %v", err)
	}
	request.PayloadSignature = base64.RawURLEncoding.EncodeToString(
		ed25519.Sign(privateKey, canonical),
	)
	return request
}
