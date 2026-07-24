package activity

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/gochya/gochya/server/internal/corebridge"
	"github.com/gochya/gochya/server/internal/dojo"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ Store = (*PostgresStore)(nil)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) (*PostgresStore, error) {
	if pool == nil {
		return nil, errors.New("PostgreSQL pool is required")
	}
	return &PostgresStore{pool: pool}, nil
}

func (s *PostgresStore) Sync(
	ctx context.Context,
	input SyncCommit,
	core corebridge.ActivityEngine,
) (SyncResponse, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return SyncResponse{}, fmt.Errorf("begin activity sync: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	timezone, streakDays, streakLastDay, err := lockActivityPlayer(
		ctx,
		tx,
		input.PlayerID,
	)
	if err != nil {
		return SyncResponse{}, err
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return SyncResponse{}, fmt.Errorf("load player timezone %q: %w", timezone, err)
	}
	snapshotTime := time.UnixMilli(int64(input.Snapshot.Timestamp))
	snapshotDay := localDay(snapshotTime, location)
	currentDay := localDay(input.Now, location)
	if !snapshotDay.Equal(currentDay) {
		return SyncResponse{}, ErrSnapshotDate
	}
	activityDate := snapshotDay.Format(time.DateOnly)

	existing, found, err := lockStoredActivity(
		ctx,
		tx,
		input.PlayerID,
		activityDate,
	)
	if err != nil {
		return SyncResponse{}, err
	}
	if found && existing.Fingerprint == input.Fingerprint {
		if err := tx.Commit(ctx); err != nil {
			return SyncResponse{}, fmt.Errorf("commit repeated activity sync: %w", err)
		}
		return SyncResponse{
			Date:             activityDate,
			Vitality:         existing.VitalityTotal,
			StatGains:        existing.StatGains,
			Goals:            existing.Goals,
			SnapshotAccepted: false,
		}, nil
	}

	petID := existing.PetID
	if !found {
		petID, err = activePetID(ctx, tx, input.PlayerID)
		if err != nil {
			return SyncResponse{}, err
		}
	}
	genomeJSON, petStats, err := lockActivityPet(
		ctx,
		tx,
		input.PlayerID,
		petID,
	)
	if err != nil {
		return SyncResponse{}, err
	}
	element, err := activityElement(genomeJSON)
	if err != nil {
		return SyncResponse{}, ErrPetStateInvalid
	}
	baseline, err := activityBaseline(ctx, tx, input.PlayerID, activityDate)
	if err != nil {
		return SyncResponse{}, err
	}
	goals, err := core.ComputeGoals(ctx, baseline)
	if err != nil {
		return SyncResponse{}, fmt.Errorf("compute activity goals: %w", err)
	}
	streakForCalculation := activityStreak(
		streakDays,
		streakLastDay,
		snapshotDay,
		found,
	)
	snapshot := input.Snapshot
	snapshot.PetElement = element
	result, err := core.ComputeActivity(ctx, snapshot, goals, streakForCalculation)
	if err != nil {
		return SyncResponse{}, fmt.Errorf("compute activity result: %w", err)
	}

	previousAwarded := existing.VitalityAwarded
	vitalityDelta := uint16(0)
	if result.Vitality > previousAwarded {
		vitalityDelta = result.Vitality - previousAwarded
	}
	newAwarded := max(previousAwarded, result.Vitality)
	totalGains := publicStatGains(result.StatGains)
	newStats, applied, statDeltas := applyStatGains(
		petStats,
		existing.Applied,
		totalGains,
	)
	if statDeltas != (StatDeltas{}) {
		statsJSON, err := json.Marshal(newStats)
		if err != nil {
			return SyncResponse{}, fmt.Errorf("encode activity pet stats: %w", err)
		}
		command, err := tx.Exec(
			ctx,
			`UPDATE pets
			    SET stats = $3
			  WHERE owner_id = $1 AND id = $2`,
			input.PlayerID,
			petID,
			statsJSON,
		)
		if err != nil {
			return SyncResponse{}, fmt.Errorf("apply activity pet stats: %w", err)
		}
		if command.RowsAffected() != 1 {
			return SyncResponse{}, ErrActivePetRequired
		}
	}
	if vitalityDelta > 0 {
		if err := applyVitality(
			ctx,
			tx,
			input.PlayerID,
			activityDate,
			vitalityDelta,
			newAwarded,
			input.Now,
		); err != nil {
			return SyncResponse{}, err
		}
	}
	if err := saveActivity(
		ctx,
		tx,
		input,
		activityDate,
		petID,
		goals,
		result.Vitality,
		newAwarded,
		totalGains,
		applied,
	); err != nil {
		return SyncResponse{}, err
	}
	if !found {
		if _, err := tx.Exec(
			ctx,
			`UPDATE players
			    SET streak_days = $2,
			        streak_last_day = $3
			  WHERE id = $1`,
			input.PlayerID,
			streakForCalculation,
			activityDate,
		); err != nil {
			return SyncResponse{}, fmt.Errorf("update activity streak: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return SyncResponse{}, fmt.Errorf("commit activity sync: %w", err)
	}
	return SyncResponse{
		Date:             activityDate,
		Vitality:         result.Vitality,
		VitalityDelta:    vitalityDelta,
		StatGains:        totalGains,
		StatGainDeltas:   statDeltas,
		Goals:            publicGoals(goals),
		SnapshotAccepted: true,
	}, nil
}

func (s *PostgresStore) Week(
	ctx context.Context,
	playerID string,
	now time.Time,
) ([]DailyActivity, error) {
	var timezone string
	err := s.pool.QueryRow(
		ctx,
		`SELECT COALESCE(NULLIF(timezone, ''), 'UTC')
		   FROM players
		  WHERE id = $1`,
		playerID,
	).Scan(&timezone)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPlayerNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query activity player timezone: %w", err)
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("load player timezone %q: %w", timezone, err)
	}
	endDay := localDay(now, location)
	startDate := endDay.AddDate(0, 0, -6).Format(time.DateOnly)
	endDate := endDay.Format(time.DateOnly)
	rows, err := s.pool.Query(
		ctx,
		`SELECT activity_date::text,
		        snapshot,
		        vitality_total,
		        vitality_awarded,
		        stat_gains,
		        goals,
		        source_metadata,
		        updated_at
		   FROM daily_activity
		  WHERE player_id = $1
		    AND activity_date BETWEEN $2::DATE AND $3::DATE
		  ORDER BY activity_date ASC`,
		playerID,
		startDate,
		endDate,
	)
	if err != nil {
		return nil, fmt.Errorf("query weekly activity: %w", err)
	}
	defer rows.Close()
	response := make([]DailyActivity, 0, 7)
	for rows.Next() {
		var (
			item          DailyActivity
			snapshotJSON  []byte
			statGainsJSON []byte
			goalsJSON     []byte
		)
		if err := rows.Scan(
			&item.Date,
			&snapshotJSON,
			&item.Vitality,
			&item.VitalityAwarded,
			&statGainsJSON,
			&goalsJSON,
			&item.SourceMetadata,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan weekly activity: %w", err)
		}
		if err := decodeStoredSnapshot(snapshotJSON, &item.Snapshot); err != nil {
			return nil, fmt.Errorf(
				"decode activity snapshot for %s: %w",
				item.Date,
				err,
			)
		}
		if err := json.Unmarshal(statGainsJSON, &item.StatGains); err != nil {
			return nil, fmt.Errorf(
				"decode activity stat gains for %s: %w",
				item.Date,
				err,
			)
		}
		if err := json.Unmarshal(goalsJSON, &item.Goals); err != nil {
			return nil, fmt.Errorf(
				"decode activity goals for %s: %w",
				item.Date,
				err,
			)
		}
		item.UpdatedAt = item.UpdatedAt.UTC()
		response = append(response, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate weekly activity: %w", err)
	}
	return response, nil
}

func (s *PostgresStore) ClaimReward(
	ctx context.Context,
	input RewardClaim,
	core corebridge.LootEngine,
) (RewardResponse, error) {
	now := input.Now.UTC().Truncate(time.Microsecond)
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return RewardResponse{}, fmt.Errorf("begin activity reward: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	timezone, _, _, err := lockActivityPlayer(ctx, tx, input.PlayerID)
	if err != nil {
		return RewardResponse{}, err
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return RewardResponse{}, fmt.Errorf("load player timezone %q: %w", timezone, err)
	}
	activityDate := localDay(now, location).Format(time.DateOnly)
	existing, found, err := lockedActivityReward(
		ctx,
		tx,
		input.PlayerID,
		activityDate,
	)
	if err != nil {
		return RewardResponse{}, err
	}
	if found {
		if err := tx.Commit(ctx); err != nil {
			return RewardResponse{}, fmt.Errorf("commit repeated activity reward: %w", err)
		}
		existing.Awarded = false
		return existing, nil
	}

	var petID string
	var vitality uint16
	err = tx.QueryRow(
		ctx,
		`SELECT pet_id::text, vitality_total
		   FROM daily_activity
		  WHERE player_id = $1 AND activity_date = $2
		  FOR UPDATE`,
		input.PlayerID,
		activityDate,
	).Scan(&petID, &vitality)
	if errors.Is(err, pgx.ErrNoRows) {
		return RewardResponse{}, ErrActivityRequired
	}
	if err != nil {
		return RewardResponse{}, fmt.Errorf("lock reward activity: %w", err)
	}
	if vitality < ActivityRewardVitality {
		return RewardResponse{}, ErrRewardLocked
	}

	var genomeJSON []byte
	err = tx.QueryRow(
		ctx,
		`SELECT genome
		   FROM pets
		  WHERE owner_id = $1 AND id = $2
		  FOR SHARE`,
		input.PlayerID,
		petID,
	).Scan(&genomeJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return RewardResponse{}, ErrActivePetRequired
	}
	if err != nil {
		return RewardResponse{}, fmt.Errorf("lock reward pet: %w", err)
	}
	element, err := activityElement(genomeJSON)
	if err != nil {
		return RewardResponse{}, ErrPetStateInvalid
	}
	stats, err := core.GenerateLootTechnique(ctx, input.Seed, 2)
	if err != nil {
		return RewardResponse{}, fmt.Errorf("generate activity reward card: %w", err)
	}
	card := dojo.TechniqueCard{
		ID:          input.CardID,
		OwnerID:     input.PlayerID,
		Type:        stats.TechniqueType,
		Element:     element,
		Rarity:      stats.Rarity,
		BaseDamage:  stats.BaseDamage,
		Speed:       stats.Speed,
		StaminaCost: stats.StaminaCost,
		CritChance:  stats.CritChance,
		Quality:     stats.Quality,
		CreatedAt:   now,
	}
	cardJSON, err := json.Marshal(card)
	if err != nil {
		return RewardResponse{}, fmt.Errorf("encode activity reward card: %w", err)
	}
	if _, err := tx.Exec(
		ctx,
		`INSERT INTO technique_cards (
		     id, owner_id, card_data, is_equipped, is_signature, created_at
		 ) VALUES ($1, $2, $3, FALSE, FALSE, $4)`,
		card.ID,
		input.PlayerID,
		cardJSON,
		now,
	); err != nil {
		return RewardResponse{}, fmt.Errorf("insert activity reward card: %w", err)
	}
	seedBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(seedBytes, input.Seed)
	if _, err := tx.Exec(
		ctx,
		`INSERT INTO activity_card_rewards (
		     player_id, activity_date, card_id, seed, created_at
		 ) VALUES ($1, $2, $3, $4, $5)`,
		input.PlayerID,
		activityDate,
		card.ID,
		seedBytes,
		now,
	); err != nil {
		return RewardResponse{}, fmt.Errorf("insert activity reward audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return RewardResponse{}, fmt.Errorf("commit activity reward: %w", err)
	}
	return RewardResponse{
		Date:    activityDate,
		Card:    card,
		Awarded: true,
	}, nil
}

func lockedActivityReward(
	ctx context.Context,
	tx pgx.Tx,
	playerID string,
	activityDate string,
) (RewardResponse, bool, error) {
	var (
		cardID    string
		cardJSON  []byte
		createdAt time.Time
		seedBytes []byte
	)
	err := tx.QueryRow(
		ctx,
		`SELECT rewards.card_id::text,
		        rewards.seed,
		        cards.card_data,
		        cards.created_at
		   FROM activity_card_rewards rewards
		   JOIN technique_cards cards ON cards.id = rewards.card_id
		  WHERE rewards.player_id = $1 AND rewards.activity_date = $2
		  FOR UPDATE OF rewards, cards`,
		playerID,
		activityDate,
	).Scan(&cardID, &seedBytes, &cardJSON, &createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return RewardResponse{}, false, nil
	}
	if err != nil {
		return RewardResponse{}, false, fmt.Errorf("lock activity reward: %w", err)
	}
	if len(seedBytes) != 8 {
		return RewardResponse{}, false, errors.New("activity reward seed is invalid")
	}
	var card dojo.TechniqueCard
	if err := json.Unmarshal(cardJSON, &card); err != nil {
		return RewardResponse{}, false, fmt.Errorf("decode activity reward card: %w", err)
	}
	card.ID = cardID
	card.OwnerID = playerID
	card.CreatedAt = createdAt.UTC()
	return RewardResponse{
		Date: activityDate,
		Card: card,
	}, true, nil
}

func decodeStoredSnapshot(data []byte, output *Snapshot) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("one JSON object is required")
	}
	if output.SchemaVersion != SnapshotSchemaVersion ||
		output.TimestampMillis <= 0 ||
		len(output.Workouts) > corebridge.MaxActivityWorkouts ||
		output.SleepQuality > 100 ||
		output.StressLevel > 100 {
		return errors.New("stored snapshot contract is invalid")
	}
	return nil
}

func lockActivityPlayer(
	ctx context.Context,
	tx pgx.Tx,
	playerID string,
) (string, uint32, string, error) {
	var timezone, streakLastDay string
	var streakDays uint32
	err := tx.QueryRow(
		ctx,
		`SELECT COALESCE(NULLIF(timezone, ''), 'UTC'),
		        streak_days,
		        COALESCE(streak_last_day::text, '')
		   FROM players
		  WHERE id = $1
		  FOR UPDATE`,
		playerID,
	).Scan(&timezone, &streakDays, &streakLastDay)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, "", ErrPlayerNotFound
	}
	if err != nil {
		return "", 0, "", fmt.Errorf("lock activity player: %w", err)
	}
	return timezone, streakDays, streakLastDay, nil
}

func lockStoredActivity(
	ctx context.Context,
	tx pgx.Tx,
	playerID string,
	activityDate string,
) (storedActivity, bool, error) {
	var (
		stored        storedActivity
		fingerprint   []byte
		statGainsJSON []byte
		appliedJSON   []byte
		goalsJSON     []byte
	)
	err := tx.QueryRow(
		ctx,
		`SELECT pet_id::text,
		        fingerprint,
		        vitality_total,
		        vitality_awarded,
		        stat_gains,
		        stat_gains_applied,
		        goals
		   FROM daily_activity
		  WHERE player_id = $1 AND activity_date = $2
		  FOR UPDATE`,
		playerID,
		activityDate,
	).Scan(
		&stored.PetID,
		&fingerprint,
		&stored.VitalityTotal,
		&stored.VitalityAwarded,
		&statGainsJSON,
		&appliedJSON,
		&goalsJSON,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return storedActivity{}, false, nil
	}
	if err != nil {
		return storedActivity{}, false, fmt.Errorf("lock daily activity: %w", err)
	}
	if len(fingerprint) != sha256Size {
		return storedActivity{}, false, errors.New("daily activity fingerprint is invalid")
	}
	copy(stored.Fingerprint[:], fingerprint)
	if err := json.Unmarshal(statGainsJSON, &stored.StatGains); err != nil {
		return storedActivity{}, false, fmt.Errorf("decode stored stat gains: %w", err)
	}
	if err := json.Unmarshal(appliedJSON, &stored.Applied); err != nil {
		return storedActivity{}, false, fmt.Errorf("decode applied stat gains: %w", err)
	}
	if err := json.Unmarshal(goalsJSON, &stored.Goals); err != nil {
		return storedActivity{}, false, fmt.Errorf("decode stored activity goals: %w", err)
	}
	return stored, true, nil
}

const sha256Size = 32

func activePetID(
	ctx context.Context,
	tx pgx.Tx,
	playerID string,
) (string, error) {
	var petID string
	err := tx.QueryRow(
		ctx,
		`SELECT id::text
		   FROM pets
		  WHERE owner_id = $1 AND is_active = TRUE
		  FOR UPDATE`,
		playerID,
	).Scan(&petID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrActivePetRequired
	}
	if err != nil {
		return "", fmt.Errorf("lock active activity pet: %w", err)
	}
	return petID, nil
}

type petStats struct {
	Strength  uint32 `json:"str"`
	Agility   uint32 `json:"agi"`
	Endurance uint32 `json:"end"`
	Focus     uint32 `json:"foc"`
}

func lockActivityPet(
	ctx context.Context,
	tx pgx.Tx,
	playerID string,
	petID string,
) ([]byte, petStats, error) {
	var genomeJSON, statsJSON []byte
	err := tx.QueryRow(
		ctx,
		`SELECT genome, stats
		   FROM pets
		  WHERE owner_id = $1 AND id = $2
		  FOR UPDATE`,
		playerID,
		petID,
	).Scan(&genomeJSON, &statsJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, petStats{}, ErrActivePetRequired
	}
	if err != nil {
		return nil, petStats{}, fmt.Errorf("lock activity pet: %w", err)
	}
	var stats petStats
	if err := json.Unmarshal(statsJSON, &stats); err != nil {
		return nil, petStats{}, fmt.Errorf("decode activity pet stats: %w", err)
	}
	return genomeJSON, stats, nil
}

func activityBaseline(
	ctx context.Context,
	tx pgx.Tx,
	playerID string,
	activityDate string,
) (corebridge.ActivityBaseline, error) {
	var steps, sleepMinutes, calories float64
	err := tx.QueryRow(
		ctx,
		`SELECT COALESCE(AVG(steps), 0)::DOUBLE PRECISION,
		        COALESCE(AVG(sleep_minutes), 0)::DOUBLE PRECISION,
		        COALESCE(AVG(active_calories), 0)::DOUBLE PRECISION
		   FROM daily_activity
		  WHERE player_id = $1
		    AND activity_date < $2::DATE
		    AND activity_date >= $2::DATE - 14`,
		playerID,
		activityDate,
	).Scan(&steps, &sleepMinutes, &calories)
	if err != nil {
		return corebridge.ActivityBaseline{}, fmt.Errorf("query activity baseline: %w", err)
	}
	return corebridge.ActivityBaseline{
		StepsAverage:          uint32(math.Round(steps)),
		SleepHoursAverage:     float32(sleepMinutes / 60),
		ActiveCaloriesAverage: uint16(math.Round(calories)),
	}, nil
}

func activityElement(data []byte) (uint8, error) {
	var genome map[string]json.RawMessage
	if err := json.Unmarshal(data, &genome); err != nil {
		return 0, err
	}
	raw, ok := genome["element"]
	if !ok {
		return 0, errors.New("pet element is missing")
	}
	var number uint8
	if json.Unmarshal(raw, &number) == nil && number <= 16 {
		return number, nil
	}
	var name string
	if json.Unmarshal(raw, &name) != nil {
		return 0, errors.New("pet element is invalid")
	}
	names := []string{
		"fire", "water", "earth", "air", "light", "dark", "arcane",
		"steam", "magma", "storm", "mud", "smoke", "sand", "eclipse",
		"inferno", "prism", "crystal",
	}
	for index, candidate := range names {
		if strings.EqualFold(name, candidate) {
			return uint8(index), nil
		}
	}
	return 0, errors.New("pet element is invalid")
}

func activityStreak(
	current uint32,
	lastDay string,
	snapshotDay time.Time,
	alreadySynced bool,
) uint32 {
	if alreadySynced || lastDay == snapshotDay.Format(time.DateOnly) {
		return current
	}
	if lastDay == snapshotDay.AddDate(0, 0, -1).Format(time.DateOnly) {
		return current + 1
	}
	return 1
}

func localDay(value time.Time, location *time.Location) time.Time {
	local := value.In(location)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
}

func applyStatGains(
	current petStats,
	previous StatGains,
	total StatGains,
) (petStats, StatGains, StatDeltas) {
	var strengthDelta, agilityDelta, enduranceDelta, focusDelta int32
	current.Strength, previous.Strength, strengthDelta = applyStat(
		current.Strength,
		previous.Strength,
		total.Strength,
	)
	current.Agility, previous.Agility, agilityDelta = applyStat(
		current.Agility,
		previous.Agility,
		total.Agility,
	)
	current.Endurance, previous.Endurance, enduranceDelta = applyStat(
		current.Endurance,
		previous.Endurance,
		total.Endurance,
	)
	current.Focus, previous.Focus, focusDelta = applyStat(
		current.Focus,
		previous.Focus,
		total.Focus,
	)
	return current, previous, StatDeltas{
		Strength:  strengthDelta,
		Agility:   agilityDelta,
		Endurance: enduranceDelta,
		Focus:     focusDelta,
	}
}

func applyStat(current uint32, previous int16, total int16) (uint32, int16, int32) {
	requested := int32(total) - int32(previous)
	if requested < 0 {
		decrease := min(uint32(-requested), current)
		actual := -int32(decrease)
		return current - decrease, int16(int32(previous) + actual), actual
	}
	increase := min(uint64(requested), uint64(^uint32(0)-current))
	actual := int32(increase)
	return current + uint32(increase), int16(int32(previous) + actual), actual
}

func applyVitality(
	ctx context.Context,
	tx pgx.Tx,
	playerID string,
	activityDate string,
	delta uint16,
	totalAwarded uint16,
	now time.Time,
) error {
	idempotencyKey := fmt.Sprintf(
		"activity:vitality:%s:%d",
		activityDate,
		totalAwarded,
	)
	if _, err := tx.Exec(
		ctx,
		`INSERT INTO transactions(
		     player_id,currency,amount,counterparty,counterparty_amount,
		     reason,ref_id,idempotency_key,created_at)
		 VALUES($1,'vitality',$3::BIGINT,'system:activity',-($3::BIGINT),
		        'activity_sync',$2,$4,$5)`,
		playerID,
		activityDate,
		delta,
		idempotencyKey,
		now,
	); err != nil {
		return fmt.Errorf("insert activity vitality ledger entry: %w", err)
	}
	if _, err := tx.Exec(
		ctx,
		`INSERT INTO player_wallet(
		     player_id,koins,vitality_daily,vitality_date,updated_at)
		 VALUES($1,0,$2,$3,$4)
		 ON CONFLICT(player_id) DO UPDATE
		 SET vitality_daily = CASE
		         WHEN player_wallet.vitality_date = EXCLUDED.vitality_date
		         THEN player_wallet.vitality_daily + EXCLUDED.vitality_daily
		         ELSE EXCLUDED.vitality_daily
		     END,
		     vitality_date = EXCLUDED.vitality_date,
		     updated_at = EXCLUDED.updated_at`,
		playerID,
		delta,
		activityDate,
		now,
	); err != nil {
		return fmt.Errorf("update activity vitality wallet: %w", err)
	}
	return nil
}

func saveActivity(
	ctx context.Context,
	tx pgx.Tx,
	input SyncCommit,
	activityDate string,
	petID string,
	goals corebridge.ActivityGoals,
	vitalityTotal uint16,
	vitalityAwarded uint16,
	statGains StatGains,
	applied StatGains,
) error {
	goalsJSON, err := json.Marshal(publicGoals(goals))
	if err != nil {
		return fmt.Errorf("encode activity goals: %w", err)
	}
	statGainsJSON, err := json.Marshal(statGains)
	if err != nil {
		return fmt.Errorf("encode activity stat gains: %w", err)
	}
	appliedJSON, err := json.Marshal(applied)
	if err != nil {
		return fmt.Errorf("encode applied activity stat gains: %w", err)
	}
	if !json.Valid(input.SnapshotJSON) ||
		bytes.Equal(bytes.TrimSpace(input.SnapshotJSON), []byte("null")) {
		return errors.New("activity snapshot JSON is invalid")
	}
	if _, err := tx.Exec(
		ctx,
		`INSERT INTO daily_activity(
		     player_id,activity_date,pet_id,snapshot,fingerprint,steps,
		     sleep_minutes,active_calories,goals,vitality_total,
		     vitality_awarded,stat_gains,stat_gains_applied,source_metadata,
		     created_at,updated_at)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$15)
		 ON CONFLICT(player_id,activity_date) DO UPDATE
		 SET snapshot=EXCLUDED.snapshot,
		     fingerprint=EXCLUDED.fingerprint,
		     steps=EXCLUDED.steps,
		     sleep_minutes=EXCLUDED.sleep_minutes,
		     active_calories=EXCLUDED.active_calories,
		     goals=EXCLUDED.goals,
		     vitality_total=EXCLUDED.vitality_total,
		     vitality_awarded=EXCLUDED.vitality_awarded,
		     stat_gains=EXCLUDED.stat_gains,
		     stat_gains_applied=EXCLUDED.stat_gains_applied,
		     source_metadata=EXCLUDED.source_metadata,
		     updated_at=EXCLUDED.updated_at`,
		input.PlayerID,
		activityDate,
		petID,
		input.SnapshotJSON,
		input.Fingerprint[:],
		input.Snapshot.Steps,
		input.Snapshot.SleepMinutes,
		input.Snapshot.ActiveCalories,
		goalsJSON,
		vitalityTotal,
		vitalityAwarded,
		statGainsJSON,
		appliedJSON,
		input.SourceMetadata,
		input.Now,
	); err != nil {
		return fmt.Errorf("save daily activity: %w", err)
	}
	return nil
}
