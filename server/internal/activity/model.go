package activity

import (
	"time"

	"github.com/gochya/gochya/server/internal/corebridge"
)

const SnapshotSchemaVersion = 1

type Workout struct {
	Kind            uint8  `json:"kind"`
	DurationMinutes uint16 `json:"durationMinutes"`
	Calories        uint16 `json:"calories"`
}

type Snapshot struct {
	SchemaVersion        uint16    `json:"schemaVersion"`
	TimestampMillis      int64     `json:"timestampMillis"`
	Steps                uint32    `json:"steps"`
	SleepMinutes         uint16    `json:"sleepMinutes"`
	SleepQuality         uint8     `json:"sleepQuality"`
	ActiveCalories       uint16    `json:"activeCalories"`
	Workouts             []Workout `json:"workouts"`
	AverageHeartRate     uint16    `json:"averageHeartRate"`
	HighHeartZoneMinutes uint16    `json:"highHeartZoneMinutes"`
	MeditationMinutes    uint16    `json:"meditationMinutes"`
	StressLevel          uint8     `json:"stressLevel"`
	Floors               uint16    `json:"floors"`
	StandHours           uint8     `json:"standHours"`
}

type SyncRequest struct {
	Snapshot       Snapshot `json:"snapshot"`
	SourceMetadata string   `json:"sourceMetadata"`
}

type StatGains struct {
	Strength  int16 `json:"str"`
	Agility   int16 `json:"agi"`
	Endurance int16 `json:"end"`
	Focus     int16 `json:"foc"`
}

type StatDeltas struct {
	Strength  int32 `json:"str"`
	Agility   int32 `json:"agi"`
	Endurance int32 `json:"end"`
	Focus     int32 `json:"foc"`
}

type Goals struct {
	Steps          uint32  `json:"steps"`
	SleepHours     float32 `json:"sleepHours"`
	ActiveCalories uint16  `json:"activeCalories"`
}

type SyncResponse struct {
	Date             string     `json:"date"`
	Vitality         uint16     `json:"vitality"`
	VitalityDelta    uint16     `json:"vitalityDelta"`
	StatGains        StatGains  `json:"statGains"`
	StatGainDeltas   StatDeltas `json:"statGainDeltas"`
	Goals            Goals      `json:"goals"`
	SnapshotAccepted bool       `json:"snapshotAccepted"`
}

type SyncCommit struct {
	PlayerID       string
	Snapshot       corebridge.ActivitySnapshot
	SnapshotJSON   []byte
	Fingerprint    [32]byte
	SourceMetadata string
	Now            time.Time
}

type storedActivity struct {
	PetID           string
	Fingerprint     [32]byte
	VitalityTotal   uint16
	VitalityAwarded uint16
	StatGains       StatGains
	Applied         StatGains
	Goals           Goals
}

func publicStatGains(input corebridge.ActivityStatGains) StatGains {
	return StatGains{
		Strength:  input.Strength,
		Agility:   input.Agility,
		Endurance: input.Endurance,
		Focus:     input.Focus,
	}
}

func publicGoals(input corebridge.ActivityGoals) Goals {
	return Goals{
		Steps:          input.Steps,
		SleepHours:     input.SleepHours,
		ActiveCalories: input.ActiveCalories,
	}
}
