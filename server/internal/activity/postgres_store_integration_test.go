package activity

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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

const activityPetID = "22222222-2222-4222-8222-222222222222"

func TestPostgresActivitySyncIsAtomicAndIdempotent(t *testing.T) {
	databaseURL := os.Getenv("GOCHYA_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("GOCHYA_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool := activityPostgresTestPool(t, ctx, databaseURL)
	now := time.Date(2026, time.July, 24, 8, 30, 0, 0, time.UTC)
	seedActivityData(t, ctx, pool, now)
	store, err := NewPostgresStore(pool)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	core := &integrationActivityCore{}
	first := activityCommit(now, 10_000, 1)

	const concurrency = 8
	responses := make([]SyncResponse, concurrency)
	failures := make([]error, concurrency)
	var group sync.WaitGroup
	for index := range responses {
		group.Add(1)
		go func() {
			defer group.Done()
			responses[index], failures[index] = store.Sync(ctx, first, core)
		}()
	}
	group.Wait()
	accepted := 0
	for index, response := range responses {
		if failures[index] != nil {
			t.Fatalf("sync %d: %v", index, failures[index])
		}
		if response.Date != "2026-07-24" ||
			response.Vitality != 100 ||
			response.StatGains != (StatGains{
				Strength: 7, Agility: 7, Endurance: 12, Focus: -5,
			}) {
			t.Fatalf("sync %d response = %#v", index, response)
		}
		if response.SnapshotAccepted {
			accepted++
			if response.VitalityDelta != 100 ||
				response.StatGainDeltas != (StatDeltas{
					Strength: 7, Agility: 7, Endurance: 12, Focus: -2,
				}) {
				t.Fatalf("accepted sync %d response = %#v", index, response)
			}
		} else if response.VitalityDelta != 0 ||
			response.StatGainDeltas != (StatDeltas{}) {
			t.Fatalf("repeated sync %d response = %#v", index, response)
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted responses = %d", accepted)
	}
	if core.goalCalls.Load() != 1 || core.activityCalls.Load() != 1 {
		t.Fatalf(
			"core calls = goals:%d activity:%d",
			core.goalCalls.Load(),
			core.activityCalls.Load(),
		)
	}
	baseline, streaks := core.observed()
	if baseline != (corebridge.ActivityBaseline{
		StepsAverage:          8_000,
		SleepHoursAverage:     7,
		ActiveCaloriesAverage: 400,
	}) || len(streaks) != 1 || streaks[0] != 5 {
		t.Fatalf("baseline/streaks = %#v/%#v", baseline, streaks)
	}
	assertActivityState(
		t,
		ctx,
		pool,
		100,
		1,
		petStats{Strength: 17, Agility: 27, Endurance: 42},
		100,
		100,
	)

	second := activityCommit(now, 15_000, 2)
	response, err := store.Sync(ctx, second, core)
	if err != nil {
		t.Fatalf("growing Sync: %v", err)
	}
	if response.Vitality != 130 ||
		response.VitalityDelta != 30 ||
		response.StatGains != (StatGains{
			Strength: 10, Agility: 9, Endurance: 15, Focus: -2,
		}) ||
		response.StatGainDeltas != (StatDeltas{
			Strength: 3, Agility: 2, Endurance: 3,
		}) {
		t.Fatalf("growing response = %#v", response)
	}
	assertActivityState(
		t,
		ctx,
		pool,
		130,
		2,
		petStats{Strength: 20, Agility: 29, Endurance: 45},
		130,
		130,
	)

	corrected := activityCommit(now, 5_000, 3)
	response, err = store.Sync(ctx, corrected, core)
	if err != nil {
		t.Fatalf("corrected Sync: %v", err)
	}
	if response.Vitality != 80 ||
		response.VitalityDelta != 0 ||
		response.StatGains != (StatGains{
			Strength: 3, Agility: 4, Endurance: 5,
		}) ||
		response.StatGainDeltas != (StatDeltas{
			Strength: -7, Agility: -5, Endurance: -10, Focus: 2,
		}) {
		t.Fatalf("corrected response = %#v", response)
	}
	assertActivityState(
		t,
		ctx,
		pool,
		130,
		2,
		petStats{Strength: 13, Agility: 24, Endurance: 35, Focus: 2},
		80,
		130,
	)
	repeated, err := store.Sync(ctx, corrected, core)
	if err != nil {
		t.Fatalf("repeat corrected Sync: %v", err)
	}
	if repeated.Vitality != 80 ||
		repeated.VitalityDelta != 0 ||
		repeated.StatGainDeltas != (StatDeltas{}) ||
		repeated.SnapshotAccepted {
		t.Fatalf("repeat corrected response = %#v", repeated)
	}
	if core.goalCalls.Load() != 3 || core.activityCalls.Load() != 3 {
		t.Fatalf(
			"final core calls = goals:%d activity:%d",
			core.goalCalls.Load(),
			core.activityCalls.Load(),
		)
	}
	week, err := store.Week(ctx, activityPlayerID, now)
	if err != nil {
		t.Fatalf("Week: %v", err)
	}
	if len(week) != 2 ||
		week[0].Date != "2026-07-23" ||
		week[0].Snapshot.Steps != 8_000 ||
		week[1].Date != "2026-07-24" ||
		week[1].Snapshot.Steps != 5_000 ||
		week[1].Vitality != 80 ||
		week[1].VitalityAwarded != 130 ||
		week[1].StatGains != (StatGains{
			Strength: 3, Agility: 4, Endurance: 5,
		}) ||
		week[1].Goals != (Goals{
			Steps: 9_200, SleepHours: 7.7, ActiveCalories: 460,
		}) ||
		week[1].SourceMetadata != "healthkit://watch" ||
		week[1].UpdatedAt.Location() != time.UTC {
		t.Fatalf("week = %#v", week)
	}
	empty, err := store.Week(ctx, activityPlayerID, now.AddDate(0, 0, 8))
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty future week = %#v, %v", empty, err)
	}
}

func TestPostgresActivityRewardIsAtomicAndNaturallyIdempotent(t *testing.T) {
	databaseURL := os.Getenv("GOCHYA_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("GOCHYA_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool := activityPostgresTestPool(t, ctx, databaseURL)
	now := time.Date(2026, time.July, 24, 8, 30, 0, 0, time.UTC)
	seedActivityData(t, ctx, pool, now)
	store, err := NewPostgresStore(pool)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	core := &integrationActivityCore{}
	if _, err := store.ClaimReward(ctx, RewardClaim{
		PlayerID: activityPlayerID,
		CardID:   "30000000-0000-4000-8000-000000000000",
		Seed:     1,
		Now:      now,
	}, core); !errors.Is(err, ErrActivityRequired) {
		t.Fatalf("missing-activity ClaimReward error = %v", err)
	}
	if _, err := store.Sync(ctx, activityCommit(now, 5_000, 1), core); err != nil {
		t.Fatalf("low-vitality Sync: %v", err)
	}
	if _, err := store.ClaimReward(ctx, RewardClaim{
		PlayerID: activityPlayerID,
		CardID:   "30000000-0000-4000-8000-000000000000",
		Seed:     1,
		Now:      now,
	}, core); !errors.Is(err, ErrRewardLocked) {
		t.Fatalf("low-vitality ClaimReward error = %v", err)
	}
	if _, err := store.Sync(ctx, activityCommit(now, 10_000, 2), core); err != nil {
		t.Fatalf("eligible Sync: %v", err)
	}

	const concurrency = 8
	responses := make([]RewardResponse, concurrency)
	failures := make([]error, concurrency)
	var group sync.WaitGroup
	for index := range responses {
		group.Add(1)
		go func() {
			defer group.Done()
			responses[index], failures[index] = store.ClaimReward(ctx, RewardClaim{
				PlayerID: activityPlayerID,
				CardID: fmt.Sprintf(
					"30000000-0000-4000-8000-%012d",
					index+1,
				),
				Seed: uint64(100 + index),
				Now:  now,
			}, core)
		}()
	}
	group.Wait()
	awarded := 0
	var awardedCardID string
	var awardedSeed uint64
	for index, response := range responses {
		if failures[index] != nil {
			t.Fatalf("claim %d: %v", index, failures[index])
		}
		if response.Date != "2026-07-24" ||
			response.Card.OwnerID != activityPlayerID ||
			response.Card.Element != 2 ||
			response.Card.Rarity != 2 ||
			response.Card.BaseDamage != 180 ||
			response.Card.Quality != 60 {
			t.Fatalf("claim %d response = %#v", index, response)
		}
		if response.Awarded {
			awarded++
			awardedCardID = response.Card.ID
			awardedSeed = uint64(100 + index)
		} else if awardedCardID != "" && response.Card.ID != awardedCardID {
			t.Fatalf("claim %d returned card %q, want %q", index, response.Card.ID, awardedCardID)
		}
	}
	if awarded != 1 || core.lootCalls.Load() != 1 {
		t.Fatalf("awarded/core calls = %d/%d", awarded, core.lootCalls.Load())
	}
	for index, response := range responses {
		if response.Card.ID != awardedCardID {
			t.Fatalf("claim %d returned card %q, want %q", index, response.Card.ID, awardedCardID)
		}
	}

	var cardCount, rewardCount int
	var storedSeed []byte
	if err := pool.QueryRow(
		ctx,
		`SELECT (SELECT COUNT(*) FROM technique_cards WHERE owner_id = $1),
		        (SELECT COUNT(*) FROM activity_card_rewards WHERE player_id = $1),
		        (SELECT seed FROM activity_card_rewards WHERE player_id = $1)`,
		activityPlayerID,
	).Scan(&cardCount, &rewardCount, &storedSeed); err != nil {
		t.Fatalf("query activity reward state: %v", err)
	}
	if cardCount != 1 ||
		rewardCount != 1 ||
		len(storedSeed) != 8 ||
		binary.BigEndian.Uint64(storedSeed) != awardedSeed {
		t.Fatalf(
			"reward state cards/rewards/seed = %d/%d/%x",
			cardCount,
			rewardCount,
			storedSeed,
		)
	}
	if _, err := store.Sync(ctx, activityCommit(now, 5_000, 3), core); err != nil {
		t.Fatalf("corrected-after-reward Sync: %v", err)
	}
	repeated, err := store.ClaimReward(ctx, RewardClaim{
		PlayerID: activityPlayerID,
		CardID:   "30000000-0000-4000-8000-999999999999",
		Seed:     999,
		Now:      now,
	}, core)
	if err != nil {
		t.Fatalf("repeat ClaimReward: %v", err)
	}
	if repeated.Awarded || repeated.Card.ID != awardedCardID || core.lootCalls.Load() != 1 {
		t.Fatalf("repeated reward/core calls = %#v/%d", repeated, core.lootCalls.Load())
	}
}

type integrationActivityCore struct {
	goalCalls     atomic.Int32
	activityCalls atomic.Int32
	lootCalls     atomic.Int32
	mu            sync.Mutex
	baseline      corebridge.ActivityBaseline
	streaks       []uint32
}

func (c *integrationActivityCore) GenerateLootTechnique(
	_ context.Context,
	_ uint64,
	maxRarity uint8,
) (corebridge.TechniqueStats, error) {
	c.lootCalls.Add(1)
	return corebridge.TechniqueStats{
		TechniqueType: 1,
		Rarity:        maxRarity,
		BaseDamage:    180,
		Speed:         65,
		StaminaCost:   9,
		CritChance:    0.12,
		Quality:       60,
	}, nil
}

func (c *integrationActivityCore) ComputeGoals(
	_ context.Context,
	baseline corebridge.ActivityBaseline,
) (corebridge.ActivityGoals, error) {
	c.goalCalls.Add(1)
	c.mu.Lock()
	c.baseline = baseline
	c.mu.Unlock()
	return corebridge.ActivityGoals{
		Steps:          9_200,
		SleepHours:     7.7,
		ActiveCalories: 460,
	}, nil
}

func (c *integrationActivityCore) ComputeActivity(
	_ context.Context,
	snapshot corebridge.ActivitySnapshot,
	_ corebridge.ActivityGoals,
	streak uint32,
) (corebridge.ActivityResult, error) {
	c.activityCalls.Add(1)
	c.mu.Lock()
	c.streaks = append(c.streaks, streak)
	c.mu.Unlock()
	switch snapshot.Steps {
	case 15_000:
		return corebridge.ActivityResult{
			Vitality: 130,
			StatGains: corebridge.ActivityStatGains{
				Strength: 10, Agility: 9, Endurance: 15, Focus: -2,
			},
		}, nil
	case 5_000:
		return corebridge.ActivityResult{
			Vitality: 80,
			StatGains: corebridge.ActivityStatGains{
				Strength: 3, Agility: 4, Endurance: 5,
			},
		}, nil
	default:
		return corebridge.ActivityResult{
			Vitality: 100,
			StatGains: corebridge.ActivityStatGains{
				Strength: 7, Agility: 7, Endurance: 12, Focus: -5,
			},
		}, nil
	}
}

func (c *integrationActivityCore) observed() (
	corebridge.ActivityBaseline,
	[]uint32,
) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.baseline, append([]uint32(nil), c.streaks...)
}

func activityCommit(now time.Time, steps uint32, fingerprint byte) SyncCommit {
	snapshotJSON, _ := json.Marshal(Snapshot{
		SchemaVersion:   SnapshotSchemaVersion,
		TimestampMillis: now.UnixMilli(),
		Steps:           steps,
	})
	return SyncCommit{
		PlayerID: activityPlayerID,
		Snapshot: corebridge.ActivitySnapshot{
			Steps:     steps,
			Timestamp: uint64(now.UnixMilli()),
		},
		SnapshotJSON:   snapshotJSON,
		Fingerprint:    [32]byte{fingerprint},
		SourceMetadata: "healthkit://watch",
		Now:            now,
	}
}

func assertActivityState(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	wallet int,
	transactionCount int,
	wantStats petStats,
	vitalityTotal int,
	vitalityAwarded int,
) {
	t.Helper()
	var (
		gotWallet       int
		ledgerAmount    int
		counterparty    int
		gotTransactions int
		statsJSON       []byte
		gotTotal        int
		gotAwarded      int
		streakDays      int
		streakLastDay   string
	)
	if err := pool.QueryRow(
		ctx,
		`SELECT vitality_daily
		   FROM player_wallet
		  WHERE player_id = $1`,
		activityPlayerID,
	).Scan(&gotWallet); err != nil {
		t.Fatalf("query activity wallet: %v", err)
	}
	if err := pool.QueryRow(
		ctx,
		`SELECT COUNT(*),
		        COALESCE(SUM(amount), 0),
		        COALESCE(SUM(counterparty_amount), 0)
		   FROM transactions
		  WHERE player_id = $1 AND currency = 'vitality'`,
		activityPlayerID,
	).Scan(&gotTransactions, &ledgerAmount, &counterparty); err != nil {
		t.Fatalf("query activity ledger: %v", err)
	}
	if err := pool.QueryRow(
		ctx,
		`SELECT stats
		   FROM pets
		  WHERE id = $1`,
		activityPetID,
	).Scan(&statsJSON); err != nil {
		t.Fatalf("query activity pet stats: %v", err)
	}
	var gotStats petStats
	if err := json.Unmarshal(statsJSON, &gotStats); err != nil {
		t.Fatalf("decode activity pet stats: %v", err)
	}
	if err := pool.QueryRow(
		ctx,
		`SELECT vitality_total, vitality_awarded
		   FROM daily_activity
		  WHERE player_id = $1 AND activity_date = '2026-07-24'`,
		activityPlayerID,
	).Scan(&gotTotal, &gotAwarded); err != nil {
		t.Fatalf("query current daily activity: %v", err)
	}
	if err := pool.QueryRow(
		ctx,
		`SELECT streak_days, streak_last_day::text
		   FROM players
		  WHERE id = $1`,
		activityPlayerID,
	).Scan(&streakDays, &streakLastDay); err != nil {
		t.Fatalf("query activity streak: %v", err)
	}
	if gotWallet != wallet ||
		gotTransactions != transactionCount ||
		ledgerAmount != wallet ||
		counterparty != -wallet ||
		gotStats != wantStats ||
		gotTotal != vitalityTotal ||
		gotAwarded != vitalityAwarded ||
		streakDays != 5 ||
		streakLastDay != "2026-07-24" {
		t.Fatalf(
			"state wallet/tx/ledger/counter/stats/totals/streak = "+
				"%d/%d/%d/%d/%#v/%d:%d/%d:%s",
			gotWallet,
			gotTransactions,
			ledgerAmount,
			counterparty,
			gotStats,
			gotTotal,
			gotAwarded,
			streakDays,
			streakLastDay,
		)
	}
}

func seedActivityData(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	now time.Time,
) {
	t.Helper()
	previousSnapshotJSON, err := json.Marshal(Snapshot{
		SchemaVersion:   SnapshotSchemaVersion,
		TimestampMillis: now.Add(-24 * time.Hour).UnixMilli(),
		Steps:           8_000,
		SleepMinutes:    420,
		ActiveCalories:  400,
		Workouts:        []Workout{},
	})
	if err != nil {
		t.Fatalf("encode previous activity snapshot: %v", err)
	}
	if _, err := pool.Exec(
		ctx,
		`INSERT INTO players(
		     id,username,auth_method,auth_subject,timezone,streak_days,
		     streak_last_day)
		 VALUES($1,'activity-player','google','activity-subject',
		        'Asia/Tbilisi',4,'2026-07-23')`,
		activityPlayerID,
	); err != nil {
		t.Fatalf("seed activity player: %v", err)
	}
	if _, err := pool.Exec(
		ctx,
		`INSERT INTO pets(
		     id,owner_id,genome,stage,needs,stats,is_active)
		 VALUES($1,$2,'{"element":"Earth"}','baby',
		        '{"hunger":80,"energy":80,"hygiene":80,"mood":80}',
		        '{"str":10,"agi":20,"end":30,"foc":2}',TRUE)`,
		activityPetID,
		activityPlayerID,
	); err != nil {
		t.Fatalf("seed activity pet: %v", err)
	}
	if _, err := pool.Exec(
		ctx,
		`INSERT INTO daily_activity(
		     player_id,activity_date,pet_id,snapshot,fingerprint,steps,
		     sleep_minutes,active_calories,goals,vitality_total,
		     vitality_awarded,stat_gains,stat_gains_applied,
		     source_metadata,created_at,updated_at)
		 VALUES($1,'2026-07-23',$2,$5,$3,8000,420,400,
		        '{"steps":9200,"sleepHours":7.7,"activeCalories":460}',
		        50,50,'{"str":1,"agi":1,"end":1,"foc":1}',
		        '{"str":1,"agi":1,"end":1,"foc":1}',
		        'healthkit://watch',$4,$4)`,
		activityPlayerID,
		activityPetID,
		make([]byte, 32),
		now.Add(-24*time.Hour),
		previousSnapshotJSON,
	); err != nil {
		t.Fatalf("seed previous daily activity: %v", err)
	}
}

func activityPostgresTestPool(
	t *testing.T,
	ctx context.Context,
	databaseURL string,
) *pgxpool.Pool {
	t.Helper()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open activity PostgreSQL admin pool: %v", err)
	}
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		admin.Close()
		t.Fatalf("generate activity schema: %v", err)
	}
	schema := "gochya_activity_" + hex.EncodeToString(randomBytes)
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		admin.Close()
		t.Fatalf("create activity schema: %v", err)
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		admin.Close()
		t.Fatalf("parse activity PostgreSQL config: %v", err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		admin.Close()
		t.Fatalf("open activity schema pool: %v", err)
	}
	migrations, err := filepath.Glob("../../migrations/*.up.sql")
	if err != nil {
		pool.Close()
		admin.Close()
		t.Fatalf("find activity migrations: %v", err)
	}
	sort.Strings(migrations)
	for _, path := range migrations {
		migration, err := os.ReadFile(path)
		if err != nil {
			pool.Close()
			admin.Close()
			t.Fatalf("read activity migration %q: %v", path, err)
		}
		if _, err := pool.Exec(ctx, string(migration)); err != nil {
			pool.Close()
			admin.Close()
			t.Fatalf("apply activity migration %q: %v", path, err)
		}
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
			t.Errorf("drop activity schema: %v", err)
		}
		admin.Close()
	})
	return pool
}
