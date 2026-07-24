package inventory

import (
	"time"

	"github.com/gochya/gochya/server/internal/dojo"
)

type ListTechniquesResponse struct {
	Items      []dojo.TechniqueCard `json:"items"`
	NextCursor string               `json:"next_cursor,omitempty"`
}

type TechniqueCursor struct {
	CreatedAt time.Time
	ID        string
}

type EquipTechniquesRequest struct {
	CardIDs      []string `json:"cardIds"`
	SignatureIdx uint8    `json:"signatureIdx"`
}

type LoadoutResponse struct {
	PetID        string    `json:"petId"`
	CardIDs      []string  `json:"cardIds"`
	SignatureIdx uint8     `json:"signatureIdx"`
	Revision     uint64    `json:"revision"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type EquipCommit struct {
	PlayerID       string
	CardIDs        []string
	SignatureIdx   uint8
	IdempotencyKey string
	RequestHash    [32]byte
	Now            time.Time
}
