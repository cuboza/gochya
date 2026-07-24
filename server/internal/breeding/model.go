package breeding

import (
	"time"

	"github.com/gochya/gochya/server/internal/corebridge"
)

const (
	BreedCostKoins = 500

	ItemLoveCrystal      = "love_crystal"
	ItemMutationCatalyst = "mutation_catalyst"
	ItemHybridCatalyst   = "hybrid_catalyst"
)

type BreedRequest struct {
	ParentAID string   `json:"parentA"`
	ParentBID string   `json:"parentB"`
	Catalysts []string `json:"catalysts"`
}

type BreedResponse struct {
	EggID         string    `json:"eggId"`
	IncubateUntil time.Time `json:"incubateUntil"`
}

type Egg struct {
	ID            string            `json:"id"`
	OwnerID       string            `json:"ownerId"`
	Genome        corebridge.Genome `json:"genome"`
	ParentAID     string            `json:"parentAId"`
	ParentBID     string            `json:"parentBId"`
	IncubateUntil time.Time         `json:"incubateUntil"`
	MutatedGenes  uint16            `json:"mutatedGenes"`
	CreatedAt     time.Time         `json:"createdAt"`
}

type Pet struct {
	ID         string            `json:"id"`
	OwnerID    string            `json:"ownerId"`
	Genome     corebridge.Genome `json:"genome"`
	Stage      string            `json:"stage"`
	Level      uint32            `json:"level"`
	XP         uint64            `json:"xp"`
	Needs      Needs             `json:"needs"`
	Stats      Stats             `json:"stats"`
	Generation uint32            `json:"generation"`
	IsActive   bool              `json:"isActive"`
	CreatedAt  time.Time         `json:"createdAt"`
	ParentAID  string            `json:"parentAId"`
	ParentBID  string            `json:"parentBId"`
	IsWeak     bool              `json:"isWeak"`
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

type BreedCommit struct {
	PlayerID         string
	ParentAID        string
	ParentBID        string
	MutationCatalyst bool
	HybridCatalyst   bool
	IdempotencyKey   string
	RequestHash      [32]byte
	EggID            string
	Seed             uint64
	Now              time.Time
}

type HatchCommit struct {
	PlayerID string
	EggID    string
	PetID    string
	Now      time.Time
}
