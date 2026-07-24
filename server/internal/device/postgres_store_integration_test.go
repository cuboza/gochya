package device

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gochya/gochya/server/internal/dojo"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresDeviceEnrollmentIsAtomic(t *testing.T) {
	databaseURL := os.Getenv("GOCHYA_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("GOCHYA_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool := devicePostgresTestPool(t, ctx, databaseURL)
	if _, err := pool.Exec(
		ctx,
		`INSERT INTO players (id) VALUES ($1)`,
		testPlayer,
	); err != nil {
		t.Fatalf("seed player: %v", err)
	}
	store, err := NewPostgresStore(pool)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	service := newTestService(
		t,
		store,
		func(context.Context, dojo.AttestationInput) error { return nil },
		now,
	)
	privateKey := testPrivateKey(8)
	preflight, err := service.Preflight(context.Background(), testPlayer, validPreflightRequest())
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	request := signedRegisterRequest(t, privateKey, preflight.Challenge)

	const concurrency = 8
	results := make([]RegisterResponse, concurrency)
	failures := make([]error, concurrency)
	var group sync.WaitGroup
	for index := range failures {
		group.Add(1)
		go func() {
			defer group.Done()
			results[index], failures[index] = service.Register(ctx, testPlayer, request)
		}()
	}
	group.Wait()

	successes := 0
	replays := 0
	for index, err := range failures {
		if err == nil {
			successes++
			if results[index].DeviceID != testDeviceID {
				t.Fatalf("result %d = %#v", index, results[index])
			}
			continue
		}
		var apiErr *Error
		if errors.As(err, &apiErr) && apiErr.Code == "enrollment_replay_detected" {
			replays++
			continue
		}
		t.Fatalf("registration %d: %v", index, err)
	}
	if successes != 1 || replays != concurrency-1 {
		t.Fatalf("successes = %d, replays = %d", successes, replays)
	}

	var storedPublicKey []byte
	var storedPlatform string
	var registeredAt time.Time
	if err := pool.QueryRow(
		ctx,
		`SELECT public_key, platform, registered_at
		   FROM registered_devices
		  WHERE player_id = $1 AND device_id = $2`,
		testPlayer,
		testDeviceID,
	).Scan(&storedPublicKey, &storedPlatform, &registeredAt); err != nil {
		t.Fatalf("query registered device: %v", err)
	}
	if !bytes.Equal(storedPublicKey, privateKey.Public().(ed25519.PublicKey)) ||
		storedPlatform != PlatformWearOS ||
		!registeredAt.Equal(now) {
		t.Fatalf(
			"stored device key/platform/time = %x/%q/%v",
			storedPublicKey,
			storedPlatform,
			registeredAt,
		)
	}

	var storedChallengeHash []byte
	var usedAt *time.Time
	if err := pool.QueryRow(
		ctx,
		`SELECT challenge_hash, used_at FROM device_enrollment_challenges`,
	).Scan(&storedChallengeHash, &usedAt); err != nil {
		t.Fatalf("query enrollment challenge: %v", err)
	}
	expectedHash := challengeDigest(preflight.Challenge)
	if !bytes.Equal(storedChallengeHash, expectedHash[:]) || usedAt == nil {
		t.Fatal("challenge was not hash-only and atomically consumed")
	}

	now = now.Add(time.Minute)
	sameKeyPreflight, err := service.Preflight(ctx, testPlayer, validPreflightRequest())
	if err != nil {
		t.Fatalf("same-key Preflight: %v", err)
	}
	sameKeyResponse, err := service.Register(
		ctx,
		testPlayer,
		signedRegisterRequest(t, privateKey, sameKeyPreflight.Challenge),
	)
	if err != nil {
		t.Fatalf("same-key Register: %v", err)
	}
	if !sameKeyResponse.RegisteredAt.Equal(registeredAt) {
		t.Fatalf(
			"same-key registration changed registeredAt from %v to %v",
			registeredAt,
			sameKeyResponse.RegisteredAt,
		)
	}

	now = now.Add(time.Minute)
	replacementPreflight, err := service.Preflight(ctx, testPlayer, validPreflightRequest())
	if err != nil {
		t.Fatalf("replacement Preflight: %v", err)
	}
	_, err = service.Register(
		ctx,
		testPlayer,
		signedRegisterRequest(t, testPrivateKey(9), replacementPreflight.Challenge),
	)
	assertErrorCode(t, err, "device_key_conflict")
	replacementHash := challengeDigest(replacementPreflight.Challenge)
	var replacementUsedAt *time.Time
	if err := pool.QueryRow(
		ctx,
		`SELECT used_at
		   FROM device_enrollment_challenges
		  WHERE challenge_hash = $1`,
		replacementHash[:],
	).Scan(&replacementUsedAt); err != nil {
		t.Fatalf("query replacement challenge: %v", err)
	}
	if replacementUsedAt != nil {
		t.Fatal("key conflict consumed its challenge")
	}

	now = now.Add(time.Minute)
	if _, err := pool.Exec(
		ctx,
		`DELETE FROM registered_devices
		  WHERE player_id = $1 AND device_id = $2`,
		testPlayer,
		testDeviceID,
	); err != nil {
		t.Fatalf("delete registered device for key race: %v", err)
	}
	firstRacePreflight, err := service.Preflight(ctx, testPlayer, validPreflightRequest())
	if err != nil {
		t.Fatalf("first race Preflight: %v", err)
	}
	secondRacePreflight, err := service.Preflight(ctx, testPlayer, validPreflightRequest())
	if err != nil {
		t.Fatalf("second race Preflight: %v", err)
	}
	raceRequests := []RegisterRequest{
		signedRegisterRequest(t, testPrivateKey(10), firstRacePreflight.Challenge),
		signedRegisterRequest(t, testPrivateKey(11), secondRacePreflight.Challenge),
	}
	raceErrors := make([]error, len(raceRequests))
	group = sync.WaitGroup{}
	for index := range raceRequests {
		group.Add(1)
		go func() {
			defer group.Done()
			_, raceErrors[index] = service.Register(ctx, testPlayer, raceRequests[index])
		}()
	}
	group.Wait()
	raceSuccesses := 0
	raceConflicts := 0
	for _, err := range raceErrors {
		if err == nil {
			raceSuccesses++
			continue
		}
		var apiErr *Error
		if errors.As(err, &apiErr) && apiErr.Code == "device_key_conflict" {
			raceConflicts++
			continue
		}
		t.Fatalf("different-key race error = %v", err)
	}
	if raceSuccesses != 1 || raceConflicts != 1 {
		t.Fatalf(
			"different-key race successes = %d, conflicts = %d",
			raceSuccesses,
			raceConflicts,
		)
	}

	downMigration, err := os.ReadFile("../../migrations/000004_device_enrollment.down.sql")
	if err != nil {
		t.Fatalf("read down migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(downMigration)); err != nil {
		t.Fatalf("apply down migration: %v", err)
	}
	var challengeTable *string
	if err := pool.QueryRow(
		ctx,
		`SELECT to_regclass('device_enrollment_challenges')::text`,
	).Scan(&challengeTable); err != nil {
		t.Fatalf("check down migration: %v", err)
	}
	if challengeTable != nil {
		t.Fatalf("down migration left table %q", *challengeTable)
	}
}

func devicePostgresTestPool(
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
	schema := "gochya_device_" + hex.EncodeToString(randomBytes)
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
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer cleanupCancel()
		if _, err := admin.Exec(
			cleanupCtx,
			"DROP SCHEMA "+identifier+" CASCADE",
		); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
		admin.Close()
	})

	const baseSchema = `
CREATE TABLE players (
    id UUID PRIMARY KEY
);
CREATE TABLE registered_devices (
    player_id       UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    device_id       TEXT NOT NULL,
    public_key      BYTEA NOT NULL CHECK (octet_length(public_key) = 32),
    platform        TEXT NOT NULL CHECK (platform IN ('wear_os', 'watch_os')),
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    registered_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    disabled_at     TIMESTAMPTZ,
    PRIMARY KEY (player_id, device_id)
);`
	if _, err := pool.Exec(ctx, baseSchema); err != nil {
		t.Fatalf("create base test schema: %v", err)
	}
	migration, err := os.ReadFile("../../migrations/000004_device_enrollment.up.sql")
	if err != nil {
		t.Fatalf("read device enrollment migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply device enrollment migration: %v", err)
	}
	return pool
}
