package battle

import (
	"time"

	"github.com/gochya/gochya/server/internal/corebridge"
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

type QueueCommit struct {
	PlayerID       string
	IdempotencyKey string
	RequestHash    [32]byte
	MatchID        string
	Seed           uint64
	Now            time.Time
}

type Simulator = corebridge.CombatEngine
