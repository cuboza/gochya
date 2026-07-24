package care

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gochya/gochya/server/internal/corebridge"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresCareReconciliationIsAtomicAndIdempotent(t *testing.T) {
	databaseURL := os.Getenv("GOCHYA_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("GOCHYA_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool := carePostgresPool(t, ctx, databaseURL)
	rawNow := time.Date(2026, time.July, 24, 12, 0, 0, 987_654_321, time.UTC)
	now := rawNow.Truncate(time.Second)
	seedCareData(t, ctx, pool, now)
	store, err := NewPostgresStore(pool)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	core := &countingCareCore{}
	commit := careCommit(now, testOpID, 0, OperationFeed, actionFeed, careItemApple, ItemApple)
	commit.Now = rawNow

	const concurrency = 8
	responses := make([]SyncResponse, concurrency)
	failures := make([]error, concurrency)
	var group sync.WaitGroup
	for index := range responses {
		group.Add(1)
		go func() {
			defer group.Done()
			responses[index], failures[index] = store.Reconcile(ctx, commit, core)
		}()
	}
	group.Wait()
	applied := 0
	alreadyApplied := 0
	for index := range responses {
		if failures[index] != nil ||
			responses[index].NewRevision != 1 ||
			len(responses[index].Results) != 1 ||
			len(responses[index].CanonicalSnapshots) != 1 ||
			responses[index].CanonicalSnapshots[0].Revision != 1 ||
			responses[index].CanonicalSnapshots[0].Needs.Hunger != 96 {
			t.Fatalf(
				"sync %d = %#v, %v",
				index,
				responses[index],
				failures[index],
			)
		}
		switch responses[index].Results[0].Status {
		case StatusApplied:
			applied++
		case StatusAlreadyApplied:
			alreadyApplied++
		default:
			t.Fatalf("sync %d status = %q", index, responses[index].Results[0].Status)
		}
	}
	if applied != 1 || alreadyApplied != concurrency-1 {
		t.Fatalf("applied/already = %d/%d", applied, alreadyApplied)
	}
	for index, response := range responses {
		if !response.ServerTime.Equal(now) ||
			!response.CanonicalSnapshots[0].NeedsUpdatedAt.Equal(now) {
			t.Fatalf(
				"sync %d timestamps = server %s needs %s",
				index,
				response.ServerTime,
				response.CanonicalSnapshots[0].NeedsUpdatedAt,
			)
		}
	}
	if core.advanceCalls.Load() != 1 || core.careCalls.Load() != 1 {
		t.Fatalf(
			"core advance/care calls = %d/%d",
			core.advanceCalls.Load(),
			core.careCalls.Load(),
		)
	}
	assertCarePersistence(t, ctx, pool, 1, 2, 1, 1, 96)

	conflict := commit
	conflict.Commands = append([]NormalizedCommand(nil), commit.Commands...)
	conflict.Commands[0].RequestHash = [32]byte{2}
	conflictResponse, err := store.Reconcile(ctx, conflict, core)
	if err != nil ||
		conflictResponse.Results[0].Status != StatusRejectedInvalid ||
		conflictResponse.Results[0].ErrorCode != "idempotency_conflict" ||
		conflictResponse.NewRevision != 1 {
		t.Fatalf("conflicting retry = %#v, %v", conflictResponse, err)
	}
	assertCarePersistence(t, ctx, pool, 1, 2, 1, 1, 96)

	concurrent := []SyncCommit{
		careCommit(
			now,
			"44444444-4444-4444-8444-444444444444",
			1,
			OperationClean,
			actionClean,
			careItemNone,
			"",
		),
		careCommit(
			now,
			"55555555-5555-4555-8555-555555555555",
			1,
			OperationPlay,
			actionPlay,
			careItemNone,
			"",
		),
	}
	concurrentResponses := make([]SyncResponse, len(concurrent))
	concurrentErrors := make([]error, len(concurrent))
	for index := range concurrent {
		group.Add(1)
		go func() {
			defer group.Done()
			concurrentResponses[index], concurrentErrors[index] = store.Reconcile(
				ctx,
				concurrent[index],
				core,
			)
		}()
	}
	group.Wait()
	applied = 0
	rejected := 0
	for index, response := range concurrentResponses {
		if concurrentErrors[index] != nil {
			t.Fatalf("concurrent care %d: %v", index, concurrentErrors[index])
		}
		switch response.Results[0].Status {
		case StatusApplied:
			applied++
		case StatusRejectedPrecondition:
			rejected++
			if response.Results[0].ErrorCode != "revision_conflict" {
				t.Fatalf("stale result = %#v", response.Results[0])
			}
		default:
			t.Fatalf("concurrent result = %#v", response.Results[0])
		}
	}
	if applied != 1 || rejected != 1 {
		t.Fatalf("concurrent applied/rejected = %d/%d", applied, rejected)
	}

	var revision uint64
	if err := pool.QueryRow(
		ctx,
		`SELECT care_revision FROM pets WHERE id=$1`,
		testPetID,
	).Scan(&revision); err != nil {
		t.Fatalf("query concurrent revision: %v", err)
	}
	if revision != 2 {
		t.Fatalf("revision after concurrent care = %d", revision)
	}

	unavailable := careCommit(
		now,
		"66666666-6666-4666-8666-666666666666",
		2,
		OperationFeed,
		actionFeed,
		careItemSteak,
		ItemSteak,
	)
	response, err := store.Reconcile(ctx, unavailable, core)
	if err != nil ||
		response.Results[0].Status != StatusRejectedPrecondition ||
		response.Results[0].ErrorCode != "item_unavailable" ||
		response.NewRevision != 2 {
		t.Fatalf("unavailable item response = %#v, %v", response, err)
	}

	expired := careCommit(
		now,
		"77777777-7777-4777-8777-777777777777",
		2,
		OperationClean,
		actionClean,
		careItemNone,
		"",
	)
	expired.Commands[0].ClientWallTime = now.Add(-25 * time.Hour)
	response, err = store.Reconcile(ctx, expired, core)
	if err != nil ||
		response.Results[0].Status != StatusRejectedExpired ||
		response.NewRevision != 2 {
		t.Fatalf("expired response = %#v, %v", response, err)
	}
	repeatedExpired, err := store.Reconcile(ctx, expired, core)
	if err != nil ||
		repeatedExpired.Results[0].Status != StatusRejectedExpired ||
		repeatedExpired.NewRevision != 2 {
		t.Fatalf("repeated expired response = %#v, %v", repeatedExpired, err)
	}

	sleep := careCommit(
		now,
		"88888888-8888-4888-8888-888888888888",
		2,
		OperationSleep,
		actionSleep,
		careItemNone,
		"",
	)
	response, err = store.Reconcile(ctx, sleep, core)
	if err != nil ||
		response.Results[0].Status != StatusApplied ||
		response.NewRevision != 3 ||
		response.CanonicalSnapshots[0].SleepingUntil == nil ||
		!response.CanonicalSnapshots[0].SleepingUntil.Equal(now.Add(sleepDuration)) {
		t.Fatalf("sleep response = %#v, %v", response, err)
	}
	secondSleep := careCommit(
		now,
		"99999999-9999-4999-8999-999999999999",
		3,
		OperationSleep,
		actionSleep,
		careItemNone,
		"",
	)
	response, err = store.Reconcile(ctx, secondSleep, core)
	if err != nil ||
		response.Results[0].Status != StatusRejectedPrecondition ||
		response.Results[0].ErrorCode != "already_sleeping" ||
		response.NewRevision != 3 {
		t.Fatalf("second sleep response = %#v, %v", response, err)
	}

	wake := careCommit(
		now.Add(4*time.Hour),
		"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		3,
		OperationPlay,
		actionPlay,
		careItemNone,
		"",
	)
	response, err = store.Reconcile(ctx, wake, core)
	if err != nil ||
		response.Results[0].Status != StatusApplied ||
		response.NewRevision != 4 ||
		response.CanonicalSnapshots[0].SleepingUntil != nil {
		t.Fatalf("wake response = %#v, %v", response, err)
	}
	if !core.sawSleepingAdvance() {
		t.Fatal("sleep interval was not advanced with sleeping=true")
	}

	lateRetry := commit
	lateRetry.Now = now.Add(4 * time.Hour)
	retryAfterLaterCare, err := store.Reconcile(ctx, lateRetry, core)
	if err != nil ||
		retryAfterLaterCare.Results[0].Status != StatusAlreadyApplied ||
		retryAfterLaterCare.NewRevision != 4 ||
		retryAfterLaterCare.CanonicalSnapshots[0].Revision != 4 {
		t.Fatalf("late retry = %#v, %v", retryAfterLaterCare, err)
	}
}

func careCommit(
	now time.Time,
	operationID string,
	baseRevision uint64,
	operationType string,
	action uint8,
	item uint8,
	itemID string,
) SyncCommit {
	hash := [32]byte{}
	copy(hash[:], []byte(operationID))
	return SyncCommit{
		PlayerID: testPlayerID,
		DeviceID: "phone-1",
		PetID:    testPetID,
		Commands: []NormalizedCommand{{
			OperationID:    operationID,
			PetID:          testPetID,
			BaseRevision:   baseRevision,
			OperationType:  operationType,
			Action:         action,
			Item:           item,
			ItemID:         itemID,
			ClientWallTime: now.Add(-time.Minute),
			RequestHash:    hash,
		}},
		Now: now,
	}
}

func assertCarePersistence(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	wantRevision uint64,
	wantItems int,
	wantOperations int,
	wantLedger int,
	wantHunger int,
) {
	t.Helper()
	var revision uint64
	var needs struct {
		Hunger int `json:"hunger"`
	}
	var items, operations, ledger int
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT care_revision FROM pets WHERE id=$1),
		(SELECT needs FROM pets WHERE id=$1),
		(SELECT quantity FROM player_items
		 WHERE player_id=$2 AND item_id='apple'),
		(SELECT COUNT(*) FROM care_operations WHERE player_id=$2),
		(SELECT COUNT(*) FROM item_transactions
		 WHERE player_id=$2 AND reason='care')`,
		testPetID,
		testPlayerID,
	).Scan(&revision, &needs, &items, &operations, &ledger); err != nil {
		t.Fatalf("query care persistence: %v", err)
	}
	if revision != wantRevision ||
		items != wantItems ||
		operations != wantOperations ||
		ledger != wantLedger ||
		needs.Hunger != wantHunger {
		t.Fatalf(
			"care persistence = revision %d items %d ops %d ledger %d hunger %d",
			revision,
			items,
			operations,
			ledger,
			needs.Hunger,
		)
	}
	var amount, counterparty int
	if err := pool.QueryRow(ctx, `SELECT COALESCE(SUM(amount),0),
		COALESCE(SUM(counterparty_amount),0)
		FROM item_transactions WHERE player_id=$1 AND reason='care'`,
		testPlayerID,
	).Scan(&amount, &counterparty); err != nil {
		t.Fatalf("query care ledger: %v", err)
	}
	if amount+counterparty != 0 {
		t.Fatalf("care ledger is not balanced: %d + %d", amount, counterparty)
	}
}

type countingCareCore struct {
	advanceCalls atomic.Int32
	careCalls    atomic.Int32
	mu           sync.Mutex
	sleepStates  []bool
}

func (c *countingCareCore) AdvanceNeeds(
	_ context.Context,
	state corebridge.NeedsState,
	elapsed uint64,
) (corebridge.NeedsState, error) {
	c.advanceCalls.Add(1)
	c.mu.Lock()
	c.sleepStates = append(c.sleepStates, state.Sleeping)
	c.mu.Unlock()
	hours := int(elapsed / 3_600)
	if state.Sleeping {
		state.Needs.Hunger = saturatingSubtract(state.Needs.Hunger, hours/3)
	} else {
		state.Needs.Hunger = saturatingSubtract(state.Needs.Hunger, hours)
		state.Needs.Energy = saturatingSubtract(state.Needs.Energy, hours*2/3)
		state.Needs.Hygiene = saturatingSubtract(state.Needs.Hygiene, hours/2)
	}
	return state, nil
}

func (c *countingCareCore) ApplyCare(
	_ context.Context,
	state corebridge.NeedsState,
	action uint8,
	item uint8,
) (corebridge.NeedsState, error) {
	c.careCalls.Add(1)
	switch {
	case action == actionFeed && item == careItemApple:
		state.Needs.Hunger = saturatingAdd(state.Needs.Hunger, 20)
	case action == actionClean:
		state.Needs.Hygiene = saturatingAdd(state.Needs.Hygiene, 20)
	case action == actionPlay:
		state.Needs.Mood = saturatingAdd(state.Needs.Mood, 20)
		state.Needs.Energy = saturatingSubtract(state.Needs.Energy, 5)
	case action == actionSleep:
	default:
		return corebridge.NeedsState{}, errors.New("unsupported fake care action")
	}
	state.Sleeping = action == actionSleep
	return state, nil
}

func (c *countingCareCore) sawSleepingAdvance() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, sleeping := range c.sleepStates {
		if sleeping {
			return true
		}
	}
	return false
}

func saturatingSubtract(value uint8, amount int) uint8 {
	if amount >= int(value) {
		return 0
	}
	return value - uint8(amount)
}

func saturatingAdd(value uint8, amount uint8) uint8 {
	total := uint16(value) + uint16(amount)
	if total > 100 {
		return 100
	}
	return uint8(total)
}

func seedCareData(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	now time.Time,
) {
	t.Helper()
	if _, err := pool.Exec(ctx, `INSERT INTO players(
		id,username,auth_method,auth_subject)
		VALUES($1,'care-player','google','care-player')`,
		testPlayerID,
	); err != nil {
		t.Fatalf("insert care player: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO pets(
		id,owner_id,genome,stage,level,xp,needs,stats,generation,
		is_active,created_at,needs_updated_at)
		VALUES($1,$2,'{}','baby',1,0,
		'{"hunger":100,"energy":100,"hygiene":100,"mood":100}',
		'{"str":1,"agi":1,"end":1,"foc":1}',0,TRUE,$3,$4)`,
		testPetID,
		testPlayerID,
		now.Add(-48*time.Hour),
		now.Add(-24*time.Hour),
	); err != nil {
		t.Fatalf("insert care pet: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO player_items(
		player_id,item_id,quantity,updated_at)
		VALUES($1,'apple',3,$2)`,
		testPlayerID,
		now,
	); err != nil {
		t.Fatalf("insert care items: %v", err)
	}
}

func carePostgresPool(
	t *testing.T,
	ctx context.Context,
	databaseURL string,
) *pgxpool.Pool {
	t.Helper()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open PostgreSQL admin pool: %v", err)
	}
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		admin.Close()
		t.Fatalf("generate care schema: %v", err)
	}
	schema := "gochya_care_" + hex.EncodeToString(randomBytes)
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		admin.Close()
		t.Fatalf("create care schema: %v", err)
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		admin.Close()
		t.Fatalf("parse PostgreSQL config: %v", err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		admin.Close()
		t.Fatalf("open care schema: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer cleanupCancel()
		_, _ = admin.Exec(cleanupCtx, "DROP SCHEMA "+identifier+" CASCADE")
		admin.Close()
	})
	migrations, err := filepath.Glob("../../migrations/*.up.sql")
	if err != nil {
		t.Fatalf("find migrations: %v", err)
	}
	sort.Strings(migrations)
	for _, migrationPath := range migrations {
		migration, err := os.ReadFile(migrationPath)
		if err != nil {
			t.Fatalf("read migration %q: %v", migrationPath, err)
		}
		if _, err := pool.Exec(ctx, string(migration)); err != nil {
			t.Fatalf("apply migration %q: %v", migrationPath, err)
		}
	}
	return pool
}
