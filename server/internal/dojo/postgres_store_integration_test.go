//go:build cgo && gochya_core

package dojo

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/gochya/gochya/server/internal/corebridge"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const postgresTestPlayer = "11111111-1111-4111-8111-111111111111"

func TestPostgresStoreConcurrentSubmitIsAtomic(t *testing.T) {
	databaseURL := os.Getenv("GOCHYA_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("GOCHYA_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	pool := postgresTestPool(t, ctx, databaseURL)
	privateKey := postgresSeedData(t, ctx, pool)
	store, err := NewPostgresStore(PostgresStoreConfig{Pool: pool})
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	service, err := NewService(ServiceConfig{
		Store:       store,
		Core:        corebridge.NativeEngine{},
		Attestation: AttestationVerifierFunc(validTestAttestation),
		AllowedAppBuilds: map[string]struct{}{
			testAppBuild: {},
		},
		AllowedClassifierVersions: map[string]struct{}{
			testClassifier: {},
		},
		Now:    func() time.Time { return now },
		Random: rand.Reader,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	preflight, err := service.Preflight(ctx, postgresTestPlayer, PreflightRequest{
		DeviceID: testDevice,
		AppBuild: testAppBuild,
	})
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	request := postgresSubmitRequest(t, privateKey, preflight, now)

	const concurrency = 8
	key := "22222222-2222-4222-8222-222222222222"
	responses := make([]SubmitResponse, concurrency)
	failures := make([]error, concurrency)
	var group sync.WaitGroup
	for index := range responses {
		group.Add(1)
		go func() {
			defer group.Done()
			responses[index], failures[index] = service.Submit(
				ctx,
				postgresTestPlayer,
				key,
				request,
			)
		}()
	}
	group.Wait()
	for index, err := range failures {
		if err != nil {
			t.Fatalf("Submit %d: %v", index, err)
		}
		if !reflect.DeepEqual(responses[0], responses[index]) {
			t.Fatalf("Submit %d returned another card", index)
		}
	}
	if responses[0].Card.Quality != 64 || responses[0].Card.Element != 2 {
		t.Fatalf("card = %#v", responses[0].Card)
	}

	assertPostgresCount(t, ctx, pool, "technique_cards", 1)
	assertPostgresCount(t, ctx, pool, "dojo_submission_audit", 1)
	assertPostgresCount(t, ctx, pool, "idempotency_results", 1)
	var storedNonceHash []byte
	var usedAt *time.Time
	if err := pool.QueryRow(
		ctx,
		`SELECT nonce_hash, used_at FROM dojo_nonces`,
	).Scan(&storedNonceHash, &usedAt); err != nil {
		t.Fatalf("query stored nonce: %v", err)
	}
	expectedHash := nonceDigest(preflight.Nonce)
	if !reflect.DeepEqual(storedNonceHash, expectedHash[:]) || usedAt == nil {
		t.Fatalf("stored nonce hash or used_at is invalid")
	}

	_, err = service.Submit(
		ctx,
		postgresTestPlayer,
		"33333333-3333-4333-8333-333333333333",
		request,
	)
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "replay_detected" {
		t.Fatalf("replay error = %v", err)
	}

	now = now.Add(30 * time.Second)
	ratePreflight, err := service.Preflight(ctx, postgresTestPlayer, PreflightRequest{
		DeviceID: testDevice,
		AppBuild: testAppBuild,
	})
	if err != nil {
		t.Fatalf("rate-limit Preflight: %v", err)
	}
	rateRequest := postgresSubmitRequest(t, privateKey, ratePreflight, now)
	rateKey := "66666666-6666-4666-8666-666666666666"
	_, err = service.Submit(ctx, postgresTestPlayer, rateKey, rateRequest)
	if !errors.As(err, &apiErr) || apiErr.Code != "rate_limited" {
		t.Fatalf("rate-limit error = %v", err)
	}
	assertPostgresCount(t, ctx, pool, "technique_cards", 1)
	rateNonceHash := nonceDigest(ratePreflight.Nonce)
	var rateUsedAt *time.Time
	if err := pool.QueryRow(
		ctx,
		`SELECT used_at FROM dojo_nonces WHERE nonce_hash = $1`,
		rateNonceHash[:],
	).Scan(&rateUsedAt); err != nil {
		t.Fatalf("query rate-limited nonce: %v", err)
	}
	if rateUsedAt != nil {
		t.Fatal("rate-limited transaction consumed its nonce")
	}

	now = now.Add(30 * time.Second)
	if _, err := service.Submit(ctx, postgresTestPlayer, rateKey, rateRequest); err != nil {
		t.Fatalf("retry after rate-limit interval: %v", err)
	}
	assertPostgresCount(t, ctx, pool, "technique_cards", 2)

	downMigration, err := os.ReadFile("../../migrations/000001_dojo.down.sql")
	if err != nil {
		t.Fatalf("read down migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(downMigration)); err != nil {
		t.Fatalf("apply down migration: %v", err)
	}
	var registeredDevicesTable *string
	if err := pool.QueryRow(
		ctx,
		`SELECT to_regclass('registered_devices')::text`,
	).Scan(&registeredDevicesTable); err != nil {
		t.Fatalf("check down migration: %v", err)
	}
	if registeredDevicesTable != nil {
		t.Fatalf("down migration left table %q", *registeredDevicesTable)
	}
}

func postgresTestPool(
	t *testing.T,
	ctx context.Context,
	databaseURL string,
) *pgxpool.Pool {
	t.Helper()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open PostgreSQL admin pool: %v", err)
	}
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		t.Fatalf("ping PostgreSQL: %v", err)
	}
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		admin.Close()
		t.Fatalf("generate schema name: %v", err)
	}
	schema := "gochya_dojo_" + hex.EncodeToString(randomBytes)
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		admin.Close()
		t.Fatalf("create test schema: %v", err)
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		admin.Close()
		t.Fatalf("parse PostgreSQL config: %v", err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	config.ConnConfig.RuntimeParams["timezone"] = "UTC"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		admin.Close()
		t.Fatalf("open schema pool: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		admin.Close()
		t.Fatalf("ping schema pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := admin.Exec(cleanupCtx, "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
		admin.Close()
	})

	const baseSchema = `
CREATE TABLE players (
    id UUID PRIMARY KEY
);
CREATE TABLE pets (
    id UUID PRIMARY KEY,
    owner_id UUID NOT NULL REFERENCES players(id),
    genome JSONB NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE technique_cards (
    id UUID PRIMARY KEY,
    owner_id UUID NOT NULL REFERENCES players(id),
    card_data JSONB NOT NULL,
    is_equipped BOOLEAN NOT NULL DEFAULT FALSE,
    is_signature BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL
);`
	if _, err := pool.Exec(ctx, baseSchema); err != nil {
		t.Fatalf("create base test schema: %v", err)
	}
	migration, err := os.ReadFile("../../migrations/000001_dojo.up.sql")
	if err != nil {
		t.Fatalf("read Dojo migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply Dojo migration: %v", err)
	}
	return pool
}

func postgresSeedData(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
) ed25519.PrivateKey {
	t.Helper()
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index + 1)
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	if _, err := pool.Exec(
		ctx,
		`INSERT INTO players (id) VALUES ($1)`,
		postgresTestPlayer,
	); err != nil {
		t.Fatalf("seed player: %v", err)
	}
	if _, err := pool.Exec(
		ctx,
		`INSERT INTO pets (id, owner_id, genome, is_active, created_at)
		 VALUES ($1, $2, $3, TRUE, NOW())`,
		"44444444-4444-4444-8444-444444444444",
		postgresTestPlayer,
		`{"element":"Earth"}`,
	); err != nil {
		t.Fatalf("seed active pet: %v", err)
	}
	if _, err := pool.Exec(
		ctx,
		`INSERT INTO registered_devices (
		     player_id, device_id, public_key, platform, enabled
		 ) VALUES ($1, $2, $3, 'wear_os', TRUE)`,
		postgresTestPlayer,
		testDevice,
		privateKey.Public().(ed25519.PublicKey),
	); err != nil {
		t.Fatalf("seed registered device: %v", err)
	}
	return privateKey
}

func postgresSubmitRequest(
	t *testing.T,
	privateKey ed25519.PrivateKey,
	preflight PreflightResponse,
	now time.Time,
) SubmitRequest {
	t.Helper()
	request := SubmitRequest{
		DeviceID:              testDevice,
		Nonce:                 preflight.Nonce,
		EvidenceSchemaVersion: preflight.EvidenceSchemaVersion,
		RecordedAtMS:          now.UnixMilli(),
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
	payload, err := CanonicalPayload(request)
	if err != nil {
		t.Fatalf("CanonicalPayload: %v", err)
	}
	request.PayloadSignature = base64.RawURLEncoding.EncodeToString(
		ed25519.Sign(privateKey, payload),
	)
	return request
}

func assertPostgresCount(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	table string,
	want int,
) {
	t.Helper()
	allowed := map[string]struct{}{
		"technique_cards":       {},
		"dojo_submission_audit": {},
		"idempotency_results":   {},
	}
	if _, ok := allowed[table]; !ok {
		t.Fatalf("table %q is not allowed", table)
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if count != want {
		t.Fatalf("%s count = %d, want %d", table, count, want)
	}
}

func TestPostgresStoredResponseJSONContract(t *testing.T) {
	response := SubmitResponse{
		Card:            TechniqueCard{ID: "55555555-5555-4555-8555-555555555555"},
		EvidenceVerdict: "VALID",
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	hash := make([]byte, 32)
	decoded, _, err := decodeStoredResult(encoded, hash)
	if err != nil {
		t.Fatalf("decodeStoredResult: %v", err)
	}
	if !reflect.DeepEqual(response, decoded) {
		t.Fatalf("decoded response = %#v", decoded)
	}
}
