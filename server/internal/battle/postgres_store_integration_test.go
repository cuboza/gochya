package battle

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gochya/gochya/server/internal/corebridge"
	"github.com/gochya/gochya/server/internal/dojo"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const opponentID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"

func TestPostgresCasualMatchIsAtomicAndIdempotent(t *testing.T) {
	url := os.Getenv("GOCHYA_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("GOCHYA_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool := battlePool(t, ctx, url)
	seedBattle(t, ctx, pool)
	store, _ := NewPostgresStore(pool)
	core := &countingCore{}
	commit := QueueCommit{
		PlayerID: battlePlayer, IdempotencyKey: "33333333-3333-4333-8333-333333333333",
		RequestHash: [32]byte{1}, MatchID: "44444444-4444-4444-8444-444444444444",
		Seed: 42, Now: time.Now().UTC(),
	}
	const concurrency = 8
	results := make([]QueueResponse, concurrency)
	errs := make([]error, concurrency)
	var group sync.WaitGroup
	for index := range results {
		group.Add(1)
		go func() {
			defer group.Done()
			results[index], errs[index] = store.QueueCasual(ctx, commit, core)
		}()
	}
	group.Wait()
	for index := range results {
		if errs[index] != nil || results[index] != results[0] {
			t.Fatalf("queue %d = %#v, %v", index, results[index], errs[index])
		}
	}
	if core.calls.Load() != 1 {
		t.Fatalf("core calls = %d", core.calls.Load())
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM matches`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("matches = %d, %v", count, err)
	}
	match, err := store.Match(ctx, battlePlayer, commit.MatchID)
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if match.Result.Seed != 42 || match.Result.Winner != "a" ||
		match.LoadoutRevisionA != 1 || match.LoadoutRevisionB != 1 {
		t.Fatalf("match = %#v", match)
	}
	if _, err := store.Match(ctx, "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", commit.MatchID); err != ErrMatchNotFound {
		t.Fatalf("outsider error = %v", err)
	}
	conflict := commit
	conflict.RequestHash = [32]byte{2}
	if _, err := store.QueueCasual(ctx, conflict, core); err != ErrIdempotencyConflict {
		t.Fatalf("conflict error = %v", err)
	}
}

type countingCore struct{ calls atomic.Int32 }

func (c *countingCore) SimulateCombat(
	_ context.Context,
	_ corebridge.CombatMatch,
	seed uint64,
) (corebridge.CombatResult, error) {
	c.calls.Add(1)
	return corebridge.CombatResult{
		Winner: 0, Seed: seed, FinalHPA: 900, FinalHPB: 0,
		Rounds: []corebridge.CombatRound{{DamageAToB: 100}},
	}, nil
}

func battlePool(t *testing.T, ctx context.Context, url string) *pgxpool.Pool {
	t.Helper()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	random := make([]byte, 8)
	_, _ = rand.Read(random)
	schema := "gochya_battle_" + hex.EncodeToString(random)
	id := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+id); err != nil {
		t.Fatal(err)
	}
	config, _ := pgxpool.ParseConfig(url)
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = admin.Exec(cleanup, "DROP SCHEMA "+id+" CASCADE")
		admin.Close()
	})
	for _, path := range []string{"../../migrations/000000_base.up.sql",
		"../../migrations/000006_loadouts.up.sql", "../../migrations/000007_profile_pets_read.up.sql",
		"../../migrations/000008_casual_matches.up.sql"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(data)); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
	return pool
}

func seedBattle(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `INSERT INTO players(id,username,auth_method,auth_subject) VALUES
		($1,'a','google','a'),($2,'b','google','b')`, battlePlayer, opponentID)
	if err != nil {
		t.Fatal(err)
	}
	players := []string{battlePlayer, opponentID}
	pets := []string{"55555555-5555-4555-8555-555555555555", "66666666-6666-4666-8666-666666666666"}
	for index, player := range players {
		element := "Fire"
		if index == 1 {
			element = "Earth"
		}
		_, err := pool.Exec(ctx, `INSERT INTO pets(id,owner_id,genome,stage,needs,stats,is_active)
			VALUES($1,$2,$3,'adult','{"hunger":80,"energy":80,"hygiene":80,"mood":100}',
			'{"str":30,"agi":30,"end":30,"foc":30}',TRUE)`,
			pets[index], player, `{"element":"`+element+`","tech_affinity":"Jab"}`)
		if err != nil {
			t.Fatal(err)
		}
		cardIDs := make([]string, 5)
		for cardIndex := range 5 {
			cardIDs[cardIndex] = formatCardID(index, cardIndex)
			card, _ := json.Marshal(dojo.TechniqueCard{Type: 0, BaseDamage: 200,
				Speed: 60, StaminaCost: 10, CritChance: .1})
			_, err := pool.Exec(ctx, `INSERT INTO technique_cards(id,owner_id,card_data)
				VALUES($1,$2,$3)`, cardIDs[cardIndex], player, card)
			if err != nil {
				t.Fatal(err)
			}
		}
		_, err = pool.Exec(ctx, `INSERT INTO player_loadouts(
			player_id,pet_id,card_ids,signature_idx,revision,updated_at)
			VALUES($1,$2,$3::uuid[],4,1,NOW())`, player, pets[index], cardIDs)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func formatCardID(player, card int) string {
	prefix := "7"
	if player == 1 {
		prefix = "8"
	}
	return prefix + "0000000-0000-4000-8000-00000000000" + string(rune('1'+card))
}
