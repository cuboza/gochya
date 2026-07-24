package care

import (
	"time"

	"github.com/gochya/gochya/server/internal/corebridge"
)

const (
	SchemaVersionV1 = 1

	StatusApplied              = "APPLIED"
	StatusAlreadyApplied       = "ALREADY_APPLIED"
	StatusRejectedPrecondition = "REJECTED_PRECONDITION"
	StatusRejectedExpired      = "REJECTED_EXPIRED"
	StatusRejectedInvalid      = "REJECTED_INVALID"

	OperationFeed  = "feed"
	OperationClean = "clean"
	OperationPlay  = "play"
	OperationSleep = "sleep"

	ItemApple       = "apple"
	ItemSteak       = "steak"
	ItemEnergyDrink = "energy_drink"
	ItemSoap        = "soap"
	ItemShampoo     = "shampoo"
)

const (
	actionFeed uint8 = iota
	actionClean
	actionPlay
	actionSleep
)

const (
	careItemNone uint8 = iota
	careItemApple
	careItemSteak
	careItemEnergyDrink
	careItemSoap
	careItemShampoo
)

type SyncRequest struct {
	DeviceID string        `json:"deviceId"`
	Commands []CareCommand `json:"commands"`
}

type CareCommand struct {
	OperationID             string        `json:"operationId"`
	AggregateType           string        `json:"aggregateType"`
	AggregateID             string        `json:"aggregateId"`
	BaseRevision            uint64        `json:"baseRevision"`
	OperationType           string        `json:"operationType"`
	Arguments               CareArguments `json:"arguments"`
	ClientWallTime          time.Time     `json:"clientWallTime"`
	ClientMonotonicOffsetMS uint64        `json:"clientMonotonicOffsetMs"`
	SchemaVersion           uint16        `json:"schemaVersion"`
}

type CareArguments struct {
	ItemID string `json:"itemId,omitempty"`
}

type PetSnapshot struct {
	ID             string           `json:"id"`
	Needs          corebridge.Needs `json:"needs"`
	Revision       uint64           `json:"revision"`
	IsWeak         bool             `json:"isWeak"`
	NeedsUpdatedAt time.Time        `json:"needsUpdatedAt"`
	NeedsZeroSince *time.Time       `json:"needsZeroSince,omitempty"`
	SleepingUntil  *time.Time       `json:"sleepingUntil,omitempty"`
}

type CommandResult struct {
	OperationID string      `json:"operationId"`
	Status      string      `json:"status"`
	ErrorCode   string      `json:"errorCode,omitempty"`
	Snapshot    PetSnapshot `json:"snapshot"`
}

type SyncResponse struct {
	Results            []CommandResult `json:"results"`
	CanonicalSnapshots []PetSnapshot   `json:"canonicalSnapshots"`
	NewRevision        uint64          `json:"newRevision"`
	ServerTime         time.Time       `json:"serverTime"`
}

type NormalizedCommand struct {
	OperationID    string
	PetID          string
	BaseRevision   uint64
	OperationType  string
	Action         uint8
	Item           uint8
	ItemID         string
	ClientWallTime time.Time
	RequestHash    [32]byte
}

type SyncCommit struct {
	PlayerID string
	DeviceID string
	PetID    string
	Commands []NormalizedCommand
	Now      time.Time
}
