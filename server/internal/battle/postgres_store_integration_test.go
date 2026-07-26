package battle

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
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
	// The replay must name both species so a client can draw the opponent
	// instead of an anonymous shape: seeded pets are Fire (0) and Earth (2).
	if match.ElementA != 0 || match.ElementB != 2 {
		t.Fatalf("match elements = %d, %d", match.ElementA, match.ElementB)
	}
	if _, err := store.Match(ctx, "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", commit.MatchID); err != ErrMatchNotFound {
		t.Fatalf("outsider error = %v", err)
	}
	conflict := commit
	conflict.RequestHash = [32]byte{2}
	if _, err := store.QueueCasual(ctx, conflict, core); err != ErrIdempotencyConflict {
		t.Fatalf("conflict error = %v", err)
	}

	confirm := ConfirmCommit{
		PlayerID: battlePlayer,
		MatchID:  commit.MatchID,
		Now:      commit.Now.Add(30 * time.Second),
	}
	confirmations := make([]ConfirmResponse, concurrency)
	confirmErrors := make([]error, concurrency)
	for index := range confirmations {
		group.Add(1)
		go func() {
			defer group.Done()
			input := confirm
			input.CardID = fmt.Sprintf(
				"30000000-0000-4000-8000-%012d",
				index+1,
			)
			input.CardSeed = uint64(100 + index)
			confirmations[index], confirmErrors[index] = store.Confirm(ctx, input, core)
		}()
	}
	group.Wait()
	for index := range confirmations {
		if confirmErrors[index] != nil ||
			!reflect.DeepEqual(confirmations[index], confirmations[0]) {
			t.Fatalf(
				"confirmation %d = %#v, %v",
				index,
				confirmations[index],
				confirmErrors[index],
			)
		}
	}
	if confirmations[0].Outcome != "win" ||
		len(confirmations[0].Rewards) != 1 ||
		confirmations[0].Rewards[0] != (Reward{Currency: "koins", Amount: casualWinKoins}) ||
		confirmations[0].Card == nil ||
		confirmations[0].Card.OwnerID != battlePlayer ||
		confirmations[0].Card.Element != 0 ||
		confirmations[0].Card.Rarity != 3 ||
		confirmations[0].Card.BaseDamage != 200 {
		t.Fatalf("winner confirmation = %#v", confirmations[0])
	}
	if core.lootCalls.Load() != 1 {
		t.Fatalf("loot core calls = %d", core.lootCalls.Load())
	}
	assertPvPCardState(t, ctx, pool, battlePlayer, confirmations[0].Card.ID, 1)
	assertRewardLedger(t, ctx, pool, battlePlayer, casualWinKoins, 1)

	loserConfirmation, err := store.Confirm(ctx, ConfirmCommit{
		PlayerID: opponentID,
		MatchID:  commit.MatchID,
		CardID:   "30000000-0000-4000-8000-100000000000",
		CardSeed: 1_000,
		Now:      confirm.Now,
	}, core)
	if err != nil ||
		loserConfirmation.Outcome != "loss" ||
		len(loserConfirmation.Rewards) != 1 ||
		loserConfirmation.Rewards[0].Amount != casualLossKoins ||
		loserConfirmation.Card != nil {
		t.Fatalf("loser confirmation = %#v, %v", loserConfirmation, err)
	}
	assertRewardLedger(t, ctx, pool, opponentID, casualLossKoins, 1)
	if _, err := store.Confirm(ctx, ConfirmCommit{
		PlayerID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
		MatchID:  commit.MatchID,
		CardID:   "30000000-0000-4000-8000-200000000000",
		CardSeed: 2_000,
		Now:      confirm.Now,
	}, core); err != ErrMatchNotFound {
		t.Fatalf("outsider confirm error = %v", err)
	}

	second := QueueCommit{
		PlayerID:       opponentID,
		IdempotencyKey: "99999999-9999-4999-8999-999999999999",
		RequestHash:    [32]byte{3},
		MatchID:        "99999999-9999-4999-8999-999999999998",
		Seed:           43,
		Now:            commit.Now.Add(time.Minute),
	}
	if _, err := store.QueueCasual(ctx, second, core); err != nil {
		t.Fatalf("queue second match: %v", err)
	}
	history, err := store.History(ctx, battlePlayer, 10)
	if err != nil {
		t.Fatalf("History player A: %v", err)
	}
	if len(history) != 2 ||
		history[0].ID != second.MatchID ||
		history[0].OpponentID != opponentID ||
		history[0].Outcome != "loss" ||
		history[1].ID != commit.MatchID ||
		history[1].Outcome != "win" {
		t.Fatalf("player A history = %#v", history)
	}
	limited, err := store.History(ctx, opponentID, 1)
	if err != nil {
		t.Fatalf("History player B: %v", err)
	}
	if len(limited) != 1 ||
		limited[0].ID != second.MatchID ||
		limited[0].OpponentID != battlePlayer ||
		limited[0].Outcome != "win" {
		t.Fatalf("player B limited history = %#v", limited)
	}
	outsider, err := store.History(
		ctx,
		"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
		10,
	)
	if err != nil || len(outsider) != 0 {
		t.Fatalf("outsider history = %#v, %v", outsider, err)
	}

	var cappedMatchID string
	for index := 3; index <= casualRewardedMatchesPerDay+1; index++ {
		cappedMatchID = fmt.Sprintf(
			"90000000-0000-4000-8000-%012d",
			index,
		)
		_, err := store.QueueCasual(ctx, QueueCommit{
			PlayerID:       battlePlayer,
			IdempotencyKey: fmt.Sprintf("91000000-0000-4000-8000-%012d", index),
			RequestHash:    [32]byte{byte(index)},
			MatchID:        cappedMatchID,
			Seed:           uint64(40 + index),
			Now:            commit.Now.Add(time.Duration(index) * time.Minute),
		}, core)
		if err != nil {
			t.Fatalf("queue capped match %d: %v", index, err)
		}
	}
	capped, err := store.Confirm(ctx, ConfirmCommit{
		PlayerID: battlePlayer,
		MatchID:  cappedMatchID,
		CardID:   "30000000-0000-4000-8000-300000000000",
		CardSeed: 3_000,
		Now:      commit.Now.Add(time.Hour),
	}, core)
	if err != nil {
		t.Fatalf("confirm capped match: %v", err)
	}
	if capped.Outcome != "win" || len(capped.Rewards) != 0 || capped.Card != nil {
		t.Fatalf("capped confirmation = %#v", capped)
	}
	cappedRetry, err := store.Confirm(ctx, ConfirmCommit{
		PlayerID: battlePlayer,
		MatchID:  cappedMatchID,
		CardID:   "30000000-0000-4000-8000-400000000000",
		CardSeed: 4_000,
		Now:      commit.Now.Add(2 * time.Hour),
	}, core)
	if err != nil || !reflect.DeepEqual(cappedRetry, capped) {
		t.Fatalf("capped retry = %#v, %v", cappedRetry, err)
	}
	assertRewardLedger(t, ctx, pool, battlePlayer, casualWinKoins, 1)
	assertPvPCardState(t, ctx, pool, battlePlayer, confirmations[0].Card.ID, 1)
}

func assertPvPCardState(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	playerID string,
	cardID string,
	wantRewardCards int,
) {
	t.Helper()
	var rewardCards, matchingCards int
	var seed []byte
	if err := pool.QueryRow(ctx, `SELECT
		COUNT(*) FILTER (WHERE card_id IS NOT NULL),
		COUNT(*) FILTER (WHERE card_id=$2),
		(SELECT card_seed FROM match_confirmations
		  WHERE player_id=$1 AND card_id=$2)
		FROM match_confirmations WHERE player_id=$1`,
		playerID,
		cardID,
	).Scan(&rewardCards, &matchingCards, &seed); err != nil {
		t.Fatalf("query PvP reward confirmations: %v", err)
	}
	if rewardCards != wantRewardCards || matchingCards != 1 || len(seed) != 8 {
		t.Fatalf(
			"PvP reward cards/matching/seed = %d/%d/%x, want %d/1/8-byte",
			rewardCards,
			matchingCards,
			seed,
			wantRewardCards,
		)
	}
}

func assertRewardLedger(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	playerID string,
	wantBalance int,
	wantTransactions int,
) {
	t.Helper()
	var balance, playerSum, counterpartySum int64
	var transactions int
	if err := pool.QueryRow(ctx, `SELECT koins FROM player_wallet WHERE player_id=$1`,
		playerID).Scan(&balance); err != nil {
		t.Fatalf("query wallet: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*),COALESCE(SUM(amount),0),
		COALESCE(SUM(counterparty_amount),0)
		FROM transactions WHERE player_id=$1 AND currency='koins'`,
		playerID).Scan(&transactions, &playerSum, &counterpartySum); err != nil {
		t.Fatalf("query reward ledger: %v", err)
	}
	if transactions != wantTransactions ||
		balance != int64(wantBalance) ||
		playerSum != int64(wantBalance) ||
		counterpartySum != -int64(wantBalance) ||
		playerSum+counterpartySum != 0 {
		t.Fatalf(
			"wallet/ledger = %d/%d/%d with %d tx, want balance %d with %d tx",
			balance,
			playerSum,
			counterpartySum,
			transactions,
			wantBalance,
			wantTransactions,
		)
	}
}

type countingCore struct {
	calls     atomic.Int32
	lootCalls atomic.Int32
}

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

func (c *countingCore) GenerateLootTechnique(
	_ context.Context,
	_ uint64,
	maxRarity uint8,
) (corebridge.TechniqueStats, error) {
	c.lootCalls.Add(1)
	return corebridge.TechniqueStats{
		TechniqueType: 1,
		Rarity:        maxRarity,
		BaseDamage:    200,
		Speed:         60,
		StaminaCost:   10,
		CritChance:    0.1,
		Quality:       75,
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
		"../../migrations/000008_casual_matches.up.sql",
		"../../migrations/000009_match_rewards.up.sql",
		"../../migrations/000012_pvp_card_rewards.up.sql"} {
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
