package battle

import (
	"time"

	"github.com/gochya/gochya/server/internal/corebridge"
	"github.com/gochya/gochya/server/internal/dojo"
)

type QueueRequest struct {
	Mode string `json:"mode"`
}

type QueueResponse struct {
	MatchID string `json:"matchId"`
	Status  string `json:"status"`
}

type Round struct {
	CardAIdx    uint8   `json:"cardAIdx"`
	CardBIdx    uint8   `json:"cardBIdx"`
	DamageAToB  uint16  `json:"damageAToB"`
	DamageBToA  uint16  `json:"damageBToA"`
	EffectKind  uint8   `json:"effectKind"`
	EffectValue float32 `json:"effectValue"`
}

type Result struct {
	Winner   string  `json:"winner"`
	Rounds   []Round `json:"rounds"`
	FinalHPA uint16  `json:"finalHpA"`
	FinalHPB uint16  `json:"finalHpB"`
	Seed     uint64  `json:"seed"`
}

type MatchResponse struct {
	ID               string    `json:"id"`
	PlayerAID        string    `json:"playerAId"`
	PlayerBID        string    `json:"playerBId"`
	Mode             string    `json:"mode"`
	LoadoutRevisionA uint64    `json:"loadoutRevisionA"`
	LoadoutRevisionB uint64    `json:"loadoutRevisionB"`
	Result           Result    `json:"result"`
	CreatedAt        time.Time `json:"createdAt"`
}

type MatchSummary struct {
	ID         string    `json:"id"`
	OpponentID string    `json:"opponentId"`
	Mode       string    `json:"mode"`
	Outcome    string    `json:"outcome"`
	CreatedAt  time.Time `json:"createdAt"`
}

type Reward struct {
	Currency string `json:"currency"`
	Amount   uint32 `json:"amount"`
}

type ConfirmResponse struct {
	MatchID     string              `json:"matchId"`
	Outcome     string              `json:"outcome"`
	Rewards     []Reward            `json:"rewards"`
	Card        *dojo.TechniqueCard `json:"card,omitempty"`
	ConfirmedAt time.Time           `json:"confirmedAt"`
}

type QueueCommit struct {
	PlayerID       string
	IdempotencyKey string
	RequestHash    [32]byte
	MatchID        string
	Seed           uint64
	Now            time.Time
}

type ConfirmCommit struct {
	PlayerID string
	MatchID  string
	CardID   string
	CardSeed uint64
	Now      time.Time
}

type Simulator = corebridge.CombatEngine

type Engine interface {
	corebridge.CombatEngine
	corebridge.LootEngine
}
