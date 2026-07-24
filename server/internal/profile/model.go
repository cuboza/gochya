package profile

import (
	"encoding/json"
	"time"
)

type PlayerProfile struct {
	ID            string     `json:"id"`
	Username      string     `json:"username"`
	DisplayName   string     `json:"displayName,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	LastSeen      *time.Time `json:"lastSeen,omitempty"`
	Timezone      string     `json:"timezone,omitempty"`
	StreakDays    uint32     `json:"streakDays"`
	StreakLastDay string     `json:"streakLastDay,omitempty"`
	ActivePetID   string     `json:"activePetId,omitempty"`
}

type Needs struct {
	Hunger  uint8 `json:"hunger"`
	Energy  uint8 `json:"energy"`
	Hygiene uint8 `json:"hygiene"`
	Mood    uint8 `json:"mood"`
}

type Stats struct {
	Strength  uint32 `json:"str"`
	Agility   uint32 `json:"agi"`
	Endurance uint32 `json:"end"`
	Focus     uint32 `json:"foc"`
}

type Pet struct {
	ID             string          `json:"id"`
	OwnerID        string          `json:"ownerId"`
	Genome         json.RawMessage `json:"genome"`
	Name           string          `json:"name,omitempty"`
	Stage          string          `json:"stage"`
	Level          uint32          `json:"level"`
	XP             uint64          `json:"xp"`
	Needs          Needs           `json:"needs"`
	Stats          Stats           `json:"stats"`
	Generation     uint32          `json:"generation"`
	IsActive       bool            `json:"isActive"`
	CreatedAt      time.Time       `json:"createdAt"`
	ParentAID      string          `json:"parentAId,omitempty"`
	ParentBID      string          `json:"parentBId,omitempty"`
	LastBredAt     *time.Time      `json:"lastBredAt,omitempty"`
	NeedsZeroSince *time.Time      `json:"needsZeroSince,omitempty"`
	IsWeak         bool            `json:"isWeak"`
}

type ActivatePetCommit struct {
	PlayerID string
	PetID    string
	Now      time.Time
}
