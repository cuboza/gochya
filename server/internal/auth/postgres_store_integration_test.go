package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const authPostgresTestPlayer = "11111111-1111-4111-8111-111111111111"

func TestPostgresRefreshRotationDetectsConcurrentReuse(t *testing.T) {
	databaseURL := os.Getenv("GOCHYA_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("GOCHYA_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool := authPostgresTestPool(t, ctx, databaseURL)
	if _, err := pool.Exec(
		ctx,
		`INSERT INTO players (
		     id, username, created_at, auth_method, auth_subject
		 ) VALUES ($1, 'existing-player', NOW(), 'google', 'existing-subject')`,
		authPostgresTestPlayer,
	); err != nil {
		t.Fatalf("seed player: %v", err)
	}
	store, err := NewPostgresRefreshTokenStore(pool)
	if err != nil {
		t.Fatalf("NewPostgresRefreshTokenStore: %v", err)
	}
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index + 1)
	}
	now := time.Now().UTC().Truncate(time.Second)
	service, err := NewService(ServiceConfig{
		Store:      store,
		KeyID:      "primary",
		PrivateKey: ed25519.NewKeyFromSeed(seed),
		Issuer:     "https://auth.gochya.test",
		Audience:   "gochya-api",
		Now:        func() time.Time { return now },
		Random:     rand.Reader,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	initial, err := service.Issue(ctx, authPostgresTestPlayer, "phone-1")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	var storedHash []byte
	if err := pool.QueryRow(
		ctx,
		`SELECT token_hash FROM refresh_tokens`,
	).Scan(&storedHash); err != nil {
		t.Fatalf("query token hash: %v", err)
	}
	if len(storedHash) != 32 || string(storedHash) == initial.RefreshToken {
		t.Fatal("refresh token was not stored as a digest")
	}

	results := make([]TokenPair, 2)
	failures := make([]error, 2)
	var group sync.WaitGroup
	for index := range results {
		group.Add(1)
		go func() {
			defer group.Done()
			results[index], failures[index] = service.Refresh(
				ctx,
				initial.RefreshToken,
			)
		}()
	}
	group.Wait()
	var successful TokenPair
	successes := 0
	reuses := 0
	for index, failure := range failures {
		switch {
		case failure == nil:
			successes++
			successful = results[index]
		case errors.Is(failure, ErrRefreshTokenReused):
			reuses++
		default:
			t.Fatalf("Refresh %d: %v", index, failure)
		}
	}
	if successes != 1 || reuses != 1 {
		t.Fatalf("refresh outcomes: successes=%d reuses=%d", successes, reuses)
	}
	var total, active, reuseEvents int
	if err := pool.QueryRow(
		ctx,
		`SELECT COUNT(*),
		        COUNT(*) FILTER (WHERE revoked_at IS NULL),
		        COUNT(*) FILTER (WHERE reuse_detected_at IS NOT NULL)
		   FROM refresh_tokens`,
	).Scan(&total, &active, &reuseEvents); err != nil {
		t.Fatalf("query refresh family: %v", err)
	}
	if total != 2 || active != 0 || reuseEvents != 1 {
		t.Fatalf(
			"refresh family: total=%d active=%d reuseEvents=%d",
			total,
			active,
			reuseEvents,
		)
	}
	if _, err := service.Refresh(ctx, successful.RefreshToken); !errors.Is(
		err,
		ErrRefreshTokenReused,
	) {
		t.Fatalf("replacement after family revocation: %v", err)
	}

	second, err := service.Issue(ctx, authPostgresTestPlayer, "phone-2")
	if err != nil {
		t.Fatalf("second Issue: %v", err)
	}
	if err := service.Logout(ctx, second.RefreshToken); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	secondHash, err := refreshTokenHash(second.RefreshToken)
	if err != nil {
		t.Fatalf("refreshTokenHash: %v", err)
	}
	var revokedAt *time.Time
	if err := pool.QueryRow(
		ctx,
		`SELECT revoked_at FROM refresh_tokens WHERE token_hash = $1`,
		secondHash[:],
	).Scan(&revokedAt); err != nil {
		t.Fatalf("query logged-out token: %v", err)
	}
	if revokedAt == nil {
		t.Fatal("logout left refresh token active")
	}

	downMigration, err := os.ReadFile("../../migrations/000002_auth_sessions.down.sql")
	if err != nil {
		t.Fatalf("read down migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(downMigration)); err != nil {
		t.Fatalf("apply down migration: %v", err)
	}
	var refreshTable *string
	if err := pool.QueryRow(
		ctx,
		`SELECT to_regclass('refresh_tokens')::text`,
	).Scan(&refreshTable); err != nil {
		t.Fatalf("check down migration: %v", err)
	}
	if refreshTable != nil {
		t.Fatalf("down migration left table %q", *refreshTable)
	}
}

func TestPostgresIdentityStoreConcurrentResolveIsStable(t *testing.T) {
	databaseURL := os.Getenv("GOCHYA_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("GOCHYA_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool := authPostgresTestPool(t, ctx, databaseURL)
	store, err := NewPostgresIdentityStore(pool)
	if err != nil {
		t.Fatalf("NewPostgresIdentityStore: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	candidates := []PlayerCandidate{
		{
			ID:          "22222222-2222-4222-8222-222222222222",
			Username:    "google_stable",
			DisplayName: "Player",
			Identity: ExternalIdentity{
				Provider: "google",
				Subject:  "google-subject",
			},
			Now: now,
		},
		{
			ID:          "33333333-3333-4333-8333-333333333333",
			Username:    "google_stable",
			DisplayName: "Player",
			Identity: ExternalIdentity{
				Provider: "google",
				Subject:  "google-subject",
			},
			Now: now,
		},
	}
	players := make([]Player, len(candidates))
	failures := make([]error, len(candidates))
	var group sync.WaitGroup
	for index := range candidates {
		group.Add(1)
		go func() {
			defer group.Done()
			players[index], failures[index] = store.Resolve(ctx, candidates[index])
		}()
	}
	group.Wait()
	for index, err := range failures {
		if err != nil {
			t.Fatalf("Resolve %d: %v", index, err)
		}
		if players[index].ID != players[0].ID {
			t.Fatalf("Resolve %d returned player %q, want %q", index, players[index].ID, players[0].ID)
		}
	}
	var count int
	if err := pool.QueryRow(
		ctx,
		`SELECT COUNT(*)
		   FROM players
		  WHERE auth_method = 'google' AND auth_subject = 'google-subject'`,
	).Scan(&count); err != nil {
		t.Fatalf("count resolved players: %v", err)
	}
	if count != 1 {
		t.Fatalf("resolved player count = %d", count)
	}
}

func TestPostgresLoginNonceStoreRejectsConcurrentReplay(t *testing.T) {
	databaseURL := os.Getenv("GOCHYA_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("GOCHYA_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool := authPostgresTestPool(t, ctx, databaseURL)
	store, err := NewPostgresLoginNonceStore(pool)
	if err != nil {
		t.Fatalf("NewPostgresLoginNonceStore: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	nonce := testAppleNonce(15)
	binding := samsungLoginBinding(
		testSamsungOpaqueValue(16),
		"https://auth.gochya.example/samsung/callback",
		testSamsungOpaqueValue(17),
	)
	record := LoginNonceRecord{
		Provider:  samsungLoginProvider,
		Nonce:     nonce,
		Binding:   binding,
		IssuedAt:  now,
		ExpiresAt: now.Add(5 * time.Minute),
	}
	if err := store.Create(ctx, record); err != nil {
		t.Fatalf("Create: %v", err)
	}
	var storedHash, storedBindingHash []byte
	if err := pool.QueryRow(
		ctx,
		`SELECT nonce_hash, binding_hash FROM auth_login_nonces`,
	).Scan(&storedHash, &storedBindingHash); err != nil {
		t.Fatalf("query login nonce: %v", err)
	}
	expectedHash := loginNonceDigest(nonce)
	expectedBindingHash := sha256.Sum256([]byte(binding))
	if string(storedHash) != string(expectedHash[:]) ||
		string(storedBindingHash) != string(expectedBindingHash[:]) ||
		string(storedHash) == nonce ||
		string(storedBindingHash) == binding {
		t.Fatal("login state or binding was not stored exclusively as its digest")
	}
	if err := store.Consume(
		ctx,
		samsungLoginProvider,
		nonce,
		samsungLoginBinding(
			testSamsungOpaqueValue(99),
			"https://auth.gochya.example/samsung/callback",
			testSamsungOpaqueValue(17),
		),
		now.Add(time.Second),
	); !errors.Is(err, ErrLoginNonceInvalid) {
		t.Fatalf("mismatched binding error = %v", err)
	}

	failures := make([]error, 2)
	var group sync.WaitGroup
	for index := range failures {
		group.Add(1)
		go func() {
			defer group.Done()
			failures[index] = store.Consume(
				ctx,
				samsungLoginProvider,
				nonce,
				binding,
				now.Add(time.Second),
			)
		}()
	}
	group.Wait()
	successes := 0
	replays := 0
	for index, failure := range failures {
		switch {
		case failure == nil:
			successes++
		case errors.Is(failure, ErrLoginNonceInvalid):
			replays++
		default:
			t.Fatalf("Consume %d: %v", index, failure)
		}
	}
	if successes != 1 || replays != 1 {
		t.Fatalf(
			"consume outcomes: successes=%d replays=%d",
			successes,
			replays,
		)
	}
	var usedAt *time.Time
	if err := pool.QueryRow(
		ctx,
		`SELECT used_at FROM auth_login_nonces`,
	).Scan(&usedAt); err != nil {
		t.Fatalf("query consumed nonce: %v", err)
	}
	if usedAt == nil {
		t.Fatal("successful consume did not mark nonce used")
	}

	downMigration, err := os.ReadFile(
		"../../migrations/000003_auth_login_nonces.down.sql",
	)
	if err != nil {
		t.Fatalf("read login-nonce down migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(downMigration)); err != nil {
		t.Fatalf("apply login-nonce down migration: %v", err)
	}
	var nonceTable *string
	if err := pool.QueryRow(
		ctx,
		`SELECT to_regclass('auth_login_nonces')::text`,
	).Scan(&nonceTable); err != nil {
		t.Fatalf("check login-nonce down migration: %v", err)
	}
	if nonceTable != nil {
		t.Fatalf("down migration left table %q", *nonceTable)
	}
}

func authPostgresTestPool(
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
	schema := "gochya_auth_" + hex.EncodeToString(randomBytes)
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
	if _, err := pool.Exec(
		ctx,
		`CREATE TABLE players (
		     id UUID PRIMARY KEY,
		     username TEXT UNIQUE NOT NULL,
		     display_name TEXT,
		     created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		     last_seen TIMESTAMPTZ,
		     auth_method TEXT NOT NULL,
		     auth_subject TEXT NOT NULL,
		     UNIQUE (auth_method, auth_subject)
		 )`,
	); err != nil {
		t.Fatalf("create players table: %v", err)
	}
	migration, err := os.ReadFile("../../migrations/000002_auth_sessions.up.sql")
	if err != nil {
		t.Fatalf("read auth migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply auth migration: %v", err)
	}
	loginNonceMigration, err := os.ReadFile(
		"../../migrations/000003_auth_login_nonces.up.sql",
	)
	if err != nil {
		t.Fatalf("read login-nonce migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(loginNonceMigration)); err != nil {
		t.Fatalf("apply login-nonce migration: %v", err)
	}
	return pool
}
