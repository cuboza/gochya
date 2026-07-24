package onboarding

import "time"

const (
	AgeBandUnder13 = "under13"
	AgeBand13Plus  = "13plus"

	AgeStatusEligible                = "eligible"
	AgeStatusParentalConsentRequired = "parental_consent_required"

	StarterElementFire  = "fire"
	StarterElementWater = "water"
	StarterElementEarth = "earth"

	starterIncubation = 5 * time.Second
)

type AgeGateRequest struct {
	BirthDate string `json:"birthDate"`
}

type AgeGateResponse struct {
	Status          string    `json:"status"`
	COPPARestricted bool      `json:"coppaRestricted"`
	RecordedAt      time.Time `json:"recordedAt"`
}

type StarterEggRequest struct {
	Element string `json:"element"`
}

type StarterEggResponse struct {
	EggID         string    `json:"eggId"`
	Element       string    `json:"element"`
	IncubateUntil time.Time `json:"incubateUntil"`
}

type AgeGateCommit struct {
	PlayerID       string
	AgeBand        string
	IdempotencyKey string
	Now            time.Time
}

type StarterEggCommit struct {
	PlayerID       string
	Element        string
	ElementID      uint8
	IdempotencyKey string
	RequestHash    [32]byte
	EggID          string
	Seed           uint64
	Now            time.Time
}
