package device

import "context"

type Store interface {
	PutChallenge(context.Context, ChallengeRecord) error
	Challenge(context.Context, string) (ChallengeRecord, error)
	CommitRegistration(context.Context, RegistrationCommit) (RegisteredDevice, error)
}
