package activity

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gochya/gochya/server/internal/corebridge"
)

const maxFutureClockSkew = 5 * time.Minute

type ServiceConfig struct {
	Store Store
	Core  corebridge.ActivityEngine
	Now   func() time.Time
}

type Service struct {
	store Store
	core  corebridge.ActivityEngine
	now   func() time.Time
}

func NewService(config ServiceConfig) (*Service, error) {
	if config.Store == nil || config.Core == nil {
		return nil, errors.New("activity store and core are required")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &Service{store: config.Store, core: config.Core, now: config.Now}, nil
}

func (s *Service) Sync(
	ctx context.Context,
	playerID string,
	request SyncRequest,
) (SyncResponse, error) {
	if strings.TrimSpace(playerID) == "" {
		return SyncResponse{}, apiError(
			"identity_invalid",
			"authenticated player is required",
			http.StatusBadRequest,
		)
	}
	source, sourceMetadata, err := activitySource(request.SourceMetadata)
	if err != nil {
		return SyncResponse{}, err
	}
	if request.Snapshot.SchemaVersion != SnapshotSchemaVersion {
		return SyncResponse{}, apiError(
			"snapshot_schema_invalid",
			"snapshot schema version is not supported",
			http.StatusBadRequest,
		)
	}
	if request.Snapshot.TimestampMillis <= 0 {
		return SyncResponse{}, apiError(
			"snapshot_timestamp_invalid",
			"snapshot timestamp is invalid",
			http.StatusBadRequest,
		)
	}
	now := s.now().UTC()
	timestamp := time.UnixMilli(request.Snapshot.TimestampMillis)
	if timestamp.After(now.Add(maxFutureClockSkew)) {
		return SyncResponse{}, apiError(
			"snapshot_timestamp_invalid",
			"snapshot timestamp is too far in the future",
			http.StatusBadRequest,
		)
	}
	if len(request.Snapshot.Workouts) > corebridge.MaxActivityWorkouts {
		return SyncResponse{}, apiError(
			"workout_count_invalid",
			"at most eight workouts are allowed",
			http.StatusBadRequest,
		)
	}
	if request.Snapshot.SleepQuality > 100 || request.Snapshot.StressLevel > 100 {
		return SyncResponse{}, apiError(
			"snapshot_value_invalid",
			"sleep quality and stress level must be between 0 and 100",
			http.StatusBadRequest,
		)
	}
	canonical := struct {
		Snapshot       Snapshot `json:"snapshot"`
		SourceMetadata string   `json:"sourceMetadata"`
	}{
		Snapshot:       request.Snapshot,
		SourceMetadata: sourceMetadata,
	}
	canonicalJSON, err := json.Marshal(canonical)
	if err != nil {
		return SyncResponse{}, asAPIError(err)
	}
	snapshotJSON, err := json.Marshal(request.Snapshot)
	if err != nil {
		return SyncResponse{}, asAPIError(err)
	}
	var workouts [corebridge.MaxActivityWorkouts]corebridge.ActivityWorkout
	for index, workout := range request.Snapshot.Workouts {
		workouts[index] = corebridge.ActivityWorkout{
			Kind:            workout.Kind,
			DurationMinutes: workout.DurationMinutes,
			Calories:        workout.Calories,
		}
	}
	response, err := s.store.Sync(ctx, SyncCommit{
		PlayerID: playerID,
		Snapshot: corebridge.ActivitySnapshot{
			Steps:                request.Snapshot.Steps,
			SleepMinutes:         request.Snapshot.SleepMinutes,
			SleepQuality:         request.Snapshot.SleepQuality,
			ActiveCalories:       request.Snapshot.ActiveCalories,
			Workouts:             workouts,
			WorkoutCount:         uint8(len(request.Snapshot.Workouts)),
			AverageHeartRate:     request.Snapshot.AverageHeartRate,
			HighHeartZoneMinutes: request.Snapshot.HighHeartZoneMinutes,
			MeditationMinutes:    request.Snapshot.MeditationMinutes,
			StressLevel:          request.Snapshot.StressLevel,
			Floors:               request.Snapshot.Floors,
			StandHours:           request.Snapshot.StandHours,
			Source:               source,
			Timestamp:            uint64(request.Snapshot.TimestampMillis),
		},
		SnapshotJSON:   snapshotJSON,
		Fingerprint:    sha256.Sum256(canonicalJSON),
		SourceMetadata: sourceMetadata,
		Now:            now,
	}, s.core)
	return response, mapStoreError(err)
}

func (s *Service) Week(
	ctx context.Context,
	playerID string,
) ([]DailyActivity, error) {
	if strings.TrimSpace(playerID) == "" {
		return nil, apiError(
			"identity_invalid",
			"authenticated player is required",
			http.StatusBadRequest,
		)
	}
	response, err := s.store.Week(ctx, playerID, s.now().UTC())
	if err != nil {
		return nil, mapStoreError(err)
	}
	if response == nil {
		response = []DailyActivity{}
	}
	return response, nil
}

func activitySource(raw string) (uint8, string, error) {
	source := strings.ToLower(strings.TrimSpace(raw))
	switch source {
	case "healthkit://watch", "samsung_health://watch":
		return 0, source, nil
	case "healthkit://phone", "health_connect://phone",
		"google_fit://phone", "samsung_health://phone":
		return 1, source, nil
	default:
		return 0, "", apiError(
			"source_metadata_invalid",
			"source metadata is not supported",
			http.StatusBadRequest,
		)
	}
}

func mapStoreError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrPlayerNotFound):
		return apiError(
			"profile_not_found",
			"player profile was not found",
			http.StatusNotFound,
		)
	case errors.Is(err, ErrActivePetRequired):
		return apiError(
			"active_pet_required",
			"an active pet is required",
			http.StatusConflict,
		)
	case errors.Is(err, ErrSnapshotDate):
		return apiError(
			"snapshot_date_invalid",
			"snapshot must belong to the current player day",
			http.StatusBadRequest,
		)
	case errors.Is(err, ErrPetStateInvalid):
		return apiError(
			"pet_state_invalid",
			"active pet state is invalid",
			http.StatusConflict,
		)
	case errors.Is(err, corebridge.ErrUnavailable):
		return apiError(
			"core_unavailable",
			"Gochya Core is unavailable",
			http.StatusServiceUnavailable,
		)
	default:
		return asAPIError(err)
	}
}
