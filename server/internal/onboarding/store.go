package onboarding

import (
	"context"

	"github.com/gochya/gochya/server/internal/corebridge"
)

type Store interface {
	RecordAgeGate(context.Context, AgeGateCommit) (AgeGateResponse, error)
	SelectStarterEgg(
		context.Context,
		StarterEggCommit,
		corebridge.StarterEngine,
	) (StarterEggResponse, error)
}
