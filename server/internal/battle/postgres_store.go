package battle

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/gochya/gochya/server/internal/corebridge"
	"github.com/gochya/gochya/server/internal/dojo"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	matchmakingAdvisoryLock     int64 = 0x474f43485941
	matchIdempotencyTTL               = 24 * time.Hour
	casualWinKoins                    = 30
	casualDrawKoins                   = 20
	casualLossKoins                   = 10
	casualRewardedMatchesPerDay       = 10
)

type PostgresStore struct{ pool *pgxpool.Pool }

func NewPostgresStore(pool *pgxpool.Pool) (*PostgresStore, error) {
	if pool == nil {
		return nil, errors.New("PostgreSQL pool is required")
	}
	return &PostgresStore{pool: pool}, nil
}

func (s *PostgresStore) QueueCasual(
	ctx context.Context,
	input QueueCommit,
	core Simulator,
) (QueueResponse, error) {
	now := input.Now.UTC().Truncate(time.Microsecond)
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return QueueResponse{}, fmt.Errorf("begin casual match: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, matchmakingAdvisoryLock); err != nil {
		return QueueResponse{}, fmt.Errorf("lock casual matcher: %w", err)
	}
	var playerLock int
	err = tx.QueryRow(ctx, `SELECT 1 FROM players WHERE id=$1 FOR UPDATE`, input.PlayerID).Scan(&playerLock)
	if errors.Is(err, pgx.ErrNoRows) {
		return QueueResponse{}, ErrPlayerNotFound
	}
	if err != nil {
		return QueueResponse{}, fmt.Errorf("lock player: %w", err)
	}
	var storedHash, storedResponse []byte
	var expiresAt time.Time
	err = tx.QueryRow(ctx, `SELECT request_hash,response_body,expires_at
		FROM match_idempotency WHERE player_id=$1 AND idempotency_key=$2 FOR UPDATE`,
		input.PlayerID, input.IdempotencyKey).Scan(&storedHash, &storedResponse, &expiresAt)
	if err == nil && now.Before(expiresAt) {
		if len(storedHash) != sha256.Size || string(storedHash) != string(input.RequestHash[:]) {
			return QueueResponse{}, ErrIdempotencyConflict
		}
		var response QueueResponse
		if err := json.Unmarshal(storedResponse, &response); err != nil {
			return QueueResponse{}, fmt.Errorf("decode stored match response: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return QueueResponse{}, fmt.Errorf("commit match retry: %w", err)
		}
		return response, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return QueueResponse{}, fmt.Errorf("query match idempotency: %w", err)
	}
	if err == nil {
		if _, err := tx.Exec(ctx, `DELETE FROM match_idempotency
			WHERE player_id=$1 AND idempotency_key=$2`, input.PlayerID, input.IdempotencyKey); err != nil {
			return QueueResponse{}, fmt.Errorf("delete expired match idempotency: %w", err)
		}
	}

	loadoutA, revisionA, snapshotA, err := lockedLoadout(ctx, tx, input.PlayerID)
	if err != nil {
		return QueueResponse{}, err
	}
	var opponentID string
	err = tx.QueryRow(ctx, `SELECT pl.player_id::text
		FROM player_loadouts pl
		JOIN pets p ON p.owner_id=pl.player_id AND p.id=pl.pet_id
		WHERE pl.player_id<>$1 AND p.is_weak=FALSE
		ORDER BY pl.updated_at ASC,pl.player_id ASC LIMIT 1`,
		input.PlayerID).Scan(&opponentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return QueueResponse{}, ErrNoOpponent
	}
	if err != nil {
		return QueueResponse{}, fmt.Errorf("select casual opponent: %w", err)
	}
	loadoutB, revisionB, snapshotB, err := lockedLoadout(ctx, tx, opponentID)
	if err != nil {
		return QueueResponse{}, err
	}
	coreResult, err := core.SimulateCombat(ctx, corebridge.CombatMatch{
		LoadoutA: loadoutA,
		LoadoutB: loadoutB,
		Mode:     0,
	}, input.Seed)
	if err != nil {
		return QueueResponse{}, err
	}
	result := publicResult(coreResult)
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return QueueResponse{}, fmt.Errorf("encode combat result: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO matches(
		id,player_a,player_b,loadout_a,loadout_b,loadout_revision_a,
		loadout_revision_b,match_seed,result,mode,created_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'casual',$10)`,
		input.MatchID, input.PlayerID, opponentID, snapshotA, snapshotB,
		revisionA, revisionB, int64(input.Seed), resultJSON, now); err != nil {
		return QueueResponse{}, fmt.Errorf("insert casual match: %w", err)
	}
	response := QueueResponse{MatchID: input.MatchID, Status: "completed"}
	responseJSON, _ := json.Marshal(response)
	if _, err := tx.Exec(ctx, `INSERT INTO match_idempotency(
		player_id,idempotency_key,request_hash,response_body,created_at,expires_at)
		VALUES($1,$2,$3,$4,$5,$6)`, input.PlayerID, input.IdempotencyKey,
		input.RequestHash[:], responseJSON, now, now.Add(matchIdempotencyTTL)); err != nil {
		return QueueResponse{}, fmt.Errorf("insert match idempotency: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return QueueResponse{}, fmt.Errorf("commit casual match: %w", err)
	}
	return response, nil
}

func (s *PostgresStore) Match(
	ctx context.Context,
	playerID string,
	matchID string,
) (MatchResponse, error) {
	var response MatchResponse
	var resultJSON []byte
	err := s.pool.QueryRow(ctx, `SELECT id::text,player_a::text,player_b::text,
		mode,loadout_revision_a,loadout_revision_b,result,created_at
		FROM matches WHERE id=$2 AND (player_a=$1 OR player_b=$1)`,
		playerID, matchID).Scan(&response.ID, &response.PlayerAID,
		&response.PlayerBID, &response.Mode, &response.LoadoutRevisionA,
		&response.LoadoutRevisionB, &resultJSON, &response.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return MatchResponse{}, ErrMatchNotFound
	}
	if err != nil {
		return MatchResponse{}, fmt.Errorf("query match: %w", err)
	}
	if err := json.Unmarshal(resultJSON, &response.Result); err != nil {
		return MatchResponse{}, fmt.Errorf("decode match result: %w", err)
	}
	response.CreatedAt = response.CreatedAt.UTC()
	return response, nil
}

func (s *PostgresStore) History(
	ctx context.Context,
	playerID string,
	limit int,
) ([]MatchSummary, error) {
	rows, err := s.pool.Query(ctx, `SELECT id::text,player_a::text,player_b::text,
		mode,result,created_at
		FROM matches WHERE player_a=$1 OR player_b=$1
		ORDER BY created_at DESC,id DESC LIMIT $2`, playerID, limit)
	if err != nil {
		return nil, fmt.Errorf("query match history: %w", err)
	}
	defer rows.Close()

	history := make([]MatchSummary, 0, limit)
	for rows.Next() {
		var summary MatchSummary
		var playerAID, playerBID string
		var resultJSON []byte
		if err := rows.Scan(
			&summary.ID,
			&playerAID,
			&playerBID,
			&summary.Mode,
			&resultJSON,
			&summary.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan match history: %w", err)
		}
		var result Result
		if err := json.Unmarshal(resultJSON, &result); err != nil {
			return nil, fmt.Errorf("decode match history result: %w", err)
		}
		switch {
		case playerID == playerAID:
			summary.OpponentID = playerBID
		case playerID == playerBID:
			summary.OpponentID = playerAID
		default:
			return nil, errors.New("match history returned a non-participant")
		}
		switch result.Winner {
		case "draw":
			summary.Outcome = "draw"
		case "a":
			if playerID == playerAID {
				summary.Outcome = "win"
			} else {
				summary.Outcome = "loss"
			}
		case "b":
			if playerID == playerBID {
				summary.Outcome = "win"
			} else {
				summary.Outcome = "loss"
			}
		default:
			return nil, errors.New("match history contains an invalid winner")
		}
		summary.CreatedAt = summary.CreatedAt.UTC()
		history = append(history, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate match history: %w", err)
	}
	return history, nil
}

func (s *PostgresStore) Confirm(
	ctx context.Context,
	input ConfirmCommit,
) (ConfirmResponse, error) {
	now := input.Now.UTC().Truncate(time.Microsecond)
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ConfirmResponse{}, fmt.Errorf("begin match confirmation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(
		ctx,
		`SELECT pg_advisory_xact_lock($1)`,
		matchmakingAdvisoryLock,
	); err != nil {
		return ConfirmResponse{}, fmt.Errorf("lock casual reward ordering: %w", err)
	}

	var playerAID, playerBID string
	var resultJSON []byte
	var matchCreatedAt time.Time
	err = tx.QueryRow(ctx, `SELECT player_a::text,player_b::text,result,created_at
		FROM matches WHERE id=$2 AND (player_a=$1 OR player_b=$1) FOR UPDATE`,
		input.PlayerID, input.MatchID).Scan(
		&playerAID,
		&playerBID,
		&resultJSON,
		&matchCreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ConfirmResponse{}, ErrMatchNotFound
	}
	if err != nil {
		return ConfirmResponse{}, fmt.Errorf("lock match confirmation: %w", err)
	}

	response, found, err := existingConfirmation(ctx, tx, input)
	if err != nil {
		return ConfirmResponse{}, err
	}
	if found {
		if err := tx.Commit(ctx); err != nil {
			return ConfirmResponse{}, fmt.Errorf("commit match confirmation retry: %w", err)
		}
		return response, nil
	}

	var result Result
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		return ConfirmResponse{}, fmt.Errorf("decode confirmation result: %w", err)
	}
	outcome, baseReward, err := casualReward(result.Winner, input.PlayerID == playerAID)
	if err != nil {
		return ConfirmResponse{}, err
	}
	dayStart := time.Date(
		matchCreatedAt.UTC().Year(),
		matchCreatedAt.UTC().Month(),
		matchCreatedAt.UTC().Day(),
		0,
		0,
		0,
		0,
		time.UTC,
	)
	var matchRank int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*)
		FROM matches
		WHERE mode='casual'
		  AND (player_a=$1 OR player_b=$1)
		  AND created_at >= $2
		  AND created_at < $3
		  AND (created_at < $4 OR (created_at = $4 AND id <= $5))`,
		input.PlayerID,
		dayStart,
		dayStart.Add(24*time.Hour),
		matchCreatedAt,
		input.MatchID,
	).Scan(&matchRank); err != nil {
		return ConfirmResponse{}, fmt.Errorf("rank rewarded casual match: %w", err)
	}
	reward := baseReward
	if matchRank > casualRewardedMatchesPerDay {
		reward = 0
	}
	if _, err := tx.Exec(ctx, `INSERT INTO match_confirmations(
		match_id,player_id,outcome,koins_awarded,confirmed_at)
		VALUES($1,$2,$3,$4,$5)`,
		input.MatchID,
		input.PlayerID,
		outcome,
		reward,
		now,
	); err != nil {
		return ConfirmResponse{}, fmt.Errorf("insert match confirmation: %w", err)
	}
	response = ConfirmResponse{
		MatchID:     input.MatchID,
		Outcome:     outcome,
		Rewards:     make([]Reward, 0, 1),
		ConfirmedAt: now,
	}
	if reward > 0 {
		response.Rewards = append(response.Rewards, Reward{
			Currency: "koins",
			Amount:   uint32(reward),
		})
		if err := applyCasualReward(
			ctx,
			tx,
			input.PlayerID,
			input.MatchID,
			reward,
			now,
		); err != nil {
			return ConfirmResponse{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return ConfirmResponse{}, fmt.Errorf("commit match confirmation: %w", err)
	}
	return response, nil
}

func existingConfirmation(
	ctx context.Context,
	tx pgx.Tx,
	input ConfirmCommit,
) (ConfirmResponse, bool, error) {
	var outcome string
	var reward uint32
	var confirmedAt time.Time
	err := tx.QueryRow(ctx, `SELECT outcome,koins_awarded,confirmed_at
		FROM match_confirmations WHERE match_id=$1 AND player_id=$2`,
		input.MatchID, input.PlayerID).Scan(&outcome, &reward, &confirmedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ConfirmResponse{}, false, nil
	}
	if err != nil {
		return ConfirmResponse{}, false, fmt.Errorf("query match confirmation: %w", err)
	}
	response := ConfirmResponse{
		MatchID:     input.MatchID,
		Outcome:     outcome,
		Rewards:     make([]Reward, 0, 1),
		ConfirmedAt: confirmedAt.UTC(),
	}
	if reward > 0 {
		response.Rewards = append(response.Rewards, Reward{
			Currency: "koins",
			Amount:   reward,
		})
	}
	return response, true, nil
}

func casualReward(winner string, playerIsA bool) (string, int, error) {
	switch winner {
	case "draw":
		return "draw", casualDrawKoins, nil
	case "a":
		if playerIsA {
			return "win", casualWinKoins, nil
		}
		return "loss", casualLossKoins, nil
	case "b":
		if playerIsA {
			return "loss", casualLossKoins, nil
		}
		return "win", casualWinKoins, nil
	default:
		return "", 0, errors.New("match confirmation contains an invalid winner")
	}
}

func applyCasualReward(
	ctx context.Context,
	tx pgx.Tx,
	playerID string,
	matchID string,
	reward int,
	now time.Time,
) error {
	if _, err := tx.Exec(ctx, `INSERT INTO transactions(
		player_id,currency,amount,counterparty,counterparty_amount,
		reason,ref_id,idempotency_key,created_at)
		VALUES($1,'koins',$3::BIGINT,'system:casual_rewards',-($3::BIGINT),
		'casual_match_reward',$2,'match_confirm:' || $2,$4)`,
		playerID, matchID, reward, now); err != nil {
		return fmt.Errorf("insert casual reward ledger entry: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO player_wallet(
		player_id,koins,vitality_daily,vitality_date,updated_at)
		VALUES($1,$2,0,$3,$4)
		ON CONFLICT(player_id) DO UPDATE
		SET koins=player_wallet.koins + EXCLUDED.koins,
		    updated_at=EXCLUDED.updated_at`,
		playerID, reward, now.Format(time.DateOnly), now); err != nil {
		return fmt.Errorf("update casual reward wallet: %w", err)
	}
	return nil
}

type snapshot struct {
	PetID        string                   `json:"petId"`
	CardIDs      []string                 `json:"cardIds"`
	SignatureIdx uint8                    `json:"signatureIdx"`
	Revision     uint64                   `json:"revision"`
	Combat       corebridge.CombatLoadout `json:"combat"`
}

func lockedLoadout(
	ctx context.Context,
	tx pgx.Tx,
	playerID string,
) (corebridge.CombatLoadout, uint64, []byte, error) {
	var petID string
	var cardIDs []string
	var signature uint8
	var revision uint64
	var genomeJSON, needsJSON, statsJSON []byte
	var weak bool
	err := tx.QueryRow(ctx, `SELECT pl.pet_id::text,pl.card_ids::text[],
		pl.signature_idx,pl.revision,p.genome,p.needs,p.stats,p.is_weak
		FROM player_loadouts pl JOIN pets p ON p.owner_id=pl.player_id AND p.id=pl.pet_id
		WHERE pl.player_id=$1 FOR UPDATE OF pl,p`, playerID).Scan(
		&petID, &cardIDs, &signature, &revision, &genomeJSON, &needsJSON, &statsJSON, &weak)
	if errors.Is(err, pgx.ErrNoRows) {
		return corebridge.CombatLoadout{}, 0, nil, ErrLoadoutRequired
	}
	if err != nil {
		return corebridge.CombatLoadout{}, 0, nil, fmt.Errorf("lock combat loadout: %w", err)
	}
	if weak {
		return corebridge.CombatLoadout{}, 0, nil, ErrPetWeak
	}
	var stats struct{ Strength, Agility, Endurance, Focus uint32 }
	var rawStats struct {
		Strength  uint32 `json:"str"`
		Agility   uint32 `json:"agi"`
		Endurance uint32 `json:"end"`
		Focus     uint32 `json:"foc"`
	}
	var needs struct {
		Mood uint8 `json:"mood"`
	}
	if json.Unmarshal(statsJSON, &rawStats) != nil || json.Unmarshal(needsJSON, &needs) != nil {
		return corebridge.CombatLoadout{}, 0, nil, errors.New("decode combat pet state")
	}
	stats.Strength, stats.Agility = rawStats.Strength, rawStats.Agility
	stats.Endurance, stats.Focus = rawStats.Endurance, rawStats.Focus
	element, affinity, err := combatGenome(genomeJSON)
	if err != nil {
		return corebridge.CombatLoadout{}, 0, nil, err
	}
	if len(cardIDs) != 5 || signature > 4 {
		return corebridge.CombatLoadout{}, 0, nil, ErrLoadoutRequired
	}
	output := corebridge.CombatLoadout{
		Stats: corebridge.CombatStats{Strength: stats.Strength, Agility: stats.Agility,
			Endurance: stats.Endurance, Focus: stats.Focus},
		Element: element, TechAffinity: affinity, PetMood: needs.Mood, SignatureIdx: signature,
	}
	for index, cardID := range cardIDs {
		var cardJSON []byte
		err := tx.QueryRow(ctx, `SELECT card_data FROM technique_cards
			WHERE owner_id=$1 AND id=$2 FOR SHARE`, playerID, cardID).Scan(&cardJSON)
		if errors.Is(err, pgx.ErrNoRows) {
			return corebridge.CombatLoadout{}, 0, nil, ErrLoadoutRequired
		}
		if err != nil {
			return corebridge.CombatLoadout{}, 0, nil, fmt.Errorf("lock combat card: %w", err)
		}
		var card dojo.TechniqueCard
		if json.Unmarshal(cardJSON, &card) != nil || card.Type > 6 || card.Effect > 5 ||
			card.BaseDamage < 0 || card.Speed < 0 || card.CritChance < 0 || card.CritChance > .35 ||
			math.IsNaN(float64(card.BaseDamage)) || math.IsInf(float64(card.BaseDamage), 0) {
			return corebridge.CombatLoadout{}, 0, nil, errors.New("invalid combat card snapshot")
		}
		output.Cards[index] = corebridge.CombatCard{BaseDamage: card.BaseDamage,
			Speed: card.Speed, CritChance: card.CritChance, StaminaCost: card.StaminaCost,
			TechniqueType: card.Type, EffectKind: card.Effect, EffectValue: card.EffectValue}
	}
	body, err := json.Marshal(snapshot{PetID: petID, CardIDs: cardIDs,
		SignatureIdx: signature, Revision: revision, Combat: output})
	return output, revision, body, err
}

func combatGenome(data []byte) (uint8, uint8, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return 0, 0, errors.New("decode combat genome")
	}
	element, ok := enumValue(raw["element"], []string{"fire", "water", "earth", "air", "light", "dark",
		"arcane", "steam", "magma", "storm", "mud", "smoke", "sand", "eclipse", "inferno", "prism", "crystal"})
	if !ok {
		return 0, 0, errors.New("invalid combat element")
	}
	affinityRaw := raw["tech_affinity"]
	if len(affinityRaw) == 0 {
		affinityRaw = raw["techAffinity"]
	}
	affinity, ok := enumValue(affinityRaw, []string{"jab", "hook", "uppercut", "cross", "kick", "elbow", "block"})
	if len(affinityRaw) == 0 {
		affinity, ok = 0, true
	}
	if !ok {
		return 0, 0, errors.New("invalid combat affinity")
	}
	return element, affinity, nil
}

func enumValue(raw json.RawMessage, names []string) (uint8, bool) {
	var number uint8
	if json.Unmarshal(raw, &number) == nil && int(number) < len(names) {
		return number, true
	}
	var name string
	if json.Unmarshal(raw, &name) == nil {
		for index, candidate := range names {
			if strings.EqualFold(name, candidate) {
				return uint8(index), true
			}
		}
	}
	return 0, false
}

func publicResult(input corebridge.CombatResult) Result {
	winners := []string{"a", "b", "draw"}
	result := Result{Winner: winners[input.Winner], FinalHPA: input.FinalHPA,
		FinalHPB: input.FinalHPB, Seed: input.Seed, Rounds: make([]Round, len(input.Rounds))}
	for index, round := range input.Rounds {
		result.Rounds[index] = Round{CardAIdx: round.CardAIdx, CardBIdx: round.CardBIdx,
			DamageAToB: round.DamageAToB, DamageBToA: round.DamageBToA,
			EffectKind: round.EffectKind, EffectValue: round.EffectValue}
	}
	return result
}
