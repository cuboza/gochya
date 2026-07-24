package activity

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/gochya/gochya/server/internal/corebridge"
)

const activityPlayerID = "11111111-1111-4111-8111-111111111111"

type fakeStore struct {
	response   SyncResponse
	err        error
	commits    []SyncCommit
	week       []DailyActivity
	weekErr    error
	weekPlayer string
	weekNow    time.Time
}

func (s *fakeStore) Sync(
	_ context.Context,
	input SyncCommit,
	_ corebridge.ActivityEngine,
) (SyncResponse, error) {
	s.commits = append(s.commits, input)
	return s.response, s.err
}

func (s *fakeStore) Week(
	_ context.Context,
	playerID string,
	now time.Time,
) ([]DailyActivity, error) {
	s.weekPlayer = playerID
	s.weekNow = now
	return s.week, s.weekErr
}

type fakeCore struct{}

func (fakeCore) ComputeGoals(
	context.Context,
	corebridge.ActivityBaseline,
) (corebridge.ActivityGoals, error) {
	return corebridge.ActivityGoals{}, nil
}

func (fakeCore) ComputeActivity(
	context.Context,
	corebridge.ActivitySnapshot,
	corebridge.ActivityGoals,
	uint32,
) (corebridge.ActivityResult, error) {
	return corebridge.ActivityResult{}, nil
}

func TestServiceNormalizesAndFingerprintsActivitySnapshot(t *testing.T) {
	now := time.Date(2026, time.July, 24, 8, 30, 0, 0, time.UTC)
	expected := SyncResponse{Date: "2026-07-24", Vitality: 104}
	store := &fakeStore{response: expected}
	service, err := NewService(ServiceConfig{
		Store: store,
		Core:  fakeCore{},
		Now:   func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	request := validSyncRequest(now)
	request.SourceMetadata = " HEALTHKIT://WATCH "

	response, err := service.Sync(context.Background(), activityPlayerID, request)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if !reflect.DeepEqual(response, expected) {
		t.Fatalf("response = %#v, want %#v", response, expected)
	}
	if len(store.commits) != 1 {
		t.Fatalf("commit count = %d", len(store.commits))
	}
	commit := store.commits[0]
	if commit.PlayerID != activityPlayerID ||
		commit.SourceMetadata != "healthkit://watch" ||
		commit.Snapshot.Source != 0 ||
		commit.Snapshot.WorkoutCount != 3 ||
		commit.Snapshot.Workouts[0].Kind != 2 ||
		commit.Snapshot.Timestamp != uint64(now.UnixMilli()) ||
		len(commit.SnapshotJSON) == 0 {
		t.Fatalf("commit = %#v", commit)
	}
	if _, err := service.Sync(context.Background(), activityPlayerID, request); err != nil {
		t.Fatalf("repeat Sync: %v", err)
	}
	if store.commits[1].Fingerprint != commit.Fingerprint {
		t.Fatal("equal normalized requests produced different fingerprints")
	}
}

func TestServiceRejectsInvalidActivitySnapshots(t *testing.T) {
	now := time.Date(2026, time.July, 24, 8, 30, 0, 0, time.UTC)
	tests := []struct {
		name   string
		player string
		mutate func(*SyncRequest)
		code   string
	}{
		{
			name:   "identity",
			player: "",
			code:   "identity_invalid",
		},
		{
			name:   "source",
			player: activityPlayerID,
			mutate: func(input *SyncRequest) {
				input.SourceMetadata = "untrusted://watch"
			},
			code: "source_metadata_invalid",
		},
		{
			name:   "schema",
			player: activityPlayerID,
			mutate: func(input *SyncRequest) {
				input.Snapshot.SchemaVersion = 2
			},
			code: "snapshot_schema_invalid",
		},
		{
			name:   "timestamp",
			player: activityPlayerID,
			mutate: func(input *SyncRequest) {
				input.Snapshot.TimestampMillis = 0
			},
			code: "snapshot_timestamp_invalid",
		},
		{
			name:   "future",
			player: activityPlayerID,
			mutate: func(input *SyncRequest) {
				input.Snapshot.TimestampMillis = now.Add(6 * time.Minute).UnixMilli()
			},
			code: "snapshot_timestamp_invalid",
		},
		{
			name:   "workout count",
			player: activityPlayerID,
			mutate: func(input *SyncRequest) {
				input.Snapshot.Workouts = make([]Workout, 9)
			},
			code: "workout_count_invalid",
		},
		{
			name:   "quality",
			player: activityPlayerID,
			mutate: func(input *SyncRequest) {
				input.Snapshot.SleepQuality = 101
			},
			code: "snapshot_value_invalid",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &fakeStore{}
			service, err := NewService(ServiceConfig{
				Store: store,
				Core:  fakeCore{},
				Now:   func() time.Time { return now },
			})
			if err != nil {
				t.Fatalf("NewService: %v", err)
			}
			request := validSyncRequest(now)
			if test.mutate != nil {
				test.mutate(&request)
			}
			_, err = service.Sync(context.Background(), test.player, request)
			assertActivityErrorCode(t, err, test.code)
			if len(store.commits) != 0 {
				t.Fatalf("invalid request reached store: %#v", store.commits)
			}
		})
	}
}

func TestServiceMapsActivityStoreErrors(t *testing.T) {
	now := time.Date(2026, time.July, 24, 8, 30, 0, 0, time.UTC)
	tests := []struct {
		storeError error
		code       string
	}{
		{ErrPlayerNotFound, "profile_not_found"},
		{ErrActivePetRequired, "active_pet_required"},
		{ErrSnapshotDate, "snapshot_date_invalid"},
		{ErrPetStateInvalid, "pet_state_invalid"},
		{corebridge.ErrUnavailable, "core_unavailable"},
		{errors.New("database failed"), "internal_error"},
	}
	for _, test := range tests {
		store := &fakeStore{err: test.storeError}
		service, err := NewService(ServiceConfig{
			Store: store,
			Core:  fakeCore{},
			Now:   func() time.Time { return now },
		})
		if err != nil {
			t.Fatalf("NewService: %v", err)
		}
		_, err = service.Sync(
			context.Background(),
			activityPlayerID,
			validSyncRequest(now),
		)
		assertActivityErrorCode(t, err, test.code)
	}
}

func TestServiceReturnsWeeklyActivity(t *testing.T) {
	now := time.Date(2026, time.July, 24, 8, 30, 0, 0, time.FixedZone("test", 3600))
	expected := []DailyActivity{{
		Date:            "2026-07-24",
		Vitality:        104,
		VitalityAwarded: 104,
	}}
	store := &fakeStore{week: expected}
	service, err := NewService(ServiceConfig{
		Store: store,
		Core:  fakeCore{},
		Now:   func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	response, err := service.Week(context.Background(), activityPlayerID)
	if err != nil {
		t.Fatalf("Week: %v", err)
	}
	if !reflect.DeepEqual(response, expected) ||
		store.weekPlayer != activityPlayerID ||
		!store.weekNow.Equal(now.UTC()) ||
		store.weekNow.Location() != time.UTC {
		t.Fatalf(
			"response/player/now = %#v/%q/%s",
			response,
			store.weekPlayer,
			store.weekNow,
		)
	}
	store.week = nil
	response, err = service.Week(context.Background(), activityPlayerID)
	if err != nil || response == nil || len(response) != 0 {
		t.Fatalf("empty week = %#v, %v", response, err)
	}
	_, err = service.Week(context.Background(), "")
	assertActivityErrorCode(t, err, "identity_invalid")
}

func validSyncRequest(now time.Time) SyncRequest {
	return SyncRequest{
		Snapshot: Snapshot{
			SchemaVersion:        SnapshotSchemaVersion,
			TimestampMillis:      now.UnixMilli(),
			Steps:                10_000,
			SleepMinutes:         480,
			SleepQuality:         100,
			ActiveCalories:       500,
			AverageHeartRate:     80,
			HighHeartZoneMinutes: 10,
			MeditationMinutes:    15,
			StressLevel:          20,
			Floors:               10,
			StandHours:           12,
			Workouts: []Workout{
				{Kind: 2, DurationMinutes: 30, Calories: 150},
				{Kind: 0, DurationMinutes: 30, Calories: 200},
				{Kind: 4, DurationMinutes: 60, Calories: 150},
			},
		},
		SourceMetadata: "healthkit://watch",
	}
}

func assertActivityErrorCode(t *testing.T, err error, want string) {
	t.Helper()
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want activity API error", err)
	}
	if apiErr.Code != want {
		t.Fatalf("error code = %q, want %q", apiErr.Code, want)
	}
}
