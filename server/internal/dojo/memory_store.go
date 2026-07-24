package dojo

import (
	"bytes"
	"context"
	"sync"
	"time"
)

type idempotencyEntry struct {
	Response    SubmitResponse
	RequestHash [32]byte
}

// MemoryStore is a concurrency-safe reference implementation for tests and local development.
// Production uses the same Store contract with a transactional PostgreSQL implementation.
type MemoryStore struct {
	mu             sync.Mutex
	devices        map[string]Device
	elements       map[string]uint8
	nonces         map[string]NonceRecord
	idempotency    map[string]idempotencyEntry
	replayHashes   map[[32]byte]time.Time
	cards          []TechniqueCard
	lastSubmission map[string]time.Time
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		devices:        make(map[string]Device),
		elements:       make(map[string]uint8),
		nonces:         make(map[string]NonceRecord),
		idempotency:    make(map[string]idempotencyEntry),
		replayHashes:   make(map[[32]byte]time.Time),
		lastSubmission: make(map[string]time.Time),
	}
}

func (s *MemoryStore) RegisterDevice(device Device) {
	s.mu.Lock()
	defer s.mu.Unlock()
	device.PublicKey = bytes.Clone(device.PublicKey)
	s.devices[deviceKey(device.PlayerID, device.ID)] = device
}

func (s *MemoryStore) SetActiveElement(playerID string, element uint8) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.elements[playerID] = element
}

func (s *MemoryStore) Device(_ context.Context, playerID, deviceID string) (Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	device, ok := s.devices[deviceKey(playerID, deviceID)]
	if !ok {
		return Device{}, ErrDeviceNotFound
	}
	device.PublicKey = bytes.Clone(device.PublicKey)
	return device, nil
}

func (s *MemoryStore) ActiveElement(_ context.Context, playerID string) (uint8, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	element, ok := s.elements[playerID]
	if !ok {
		return 0, ErrDeviceNotFound
	}
	return element, nil
}

func (s *MemoryStore) PutNonce(_ context.Context, nonce NonceRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nonces[nonce.Value] = nonce
	return nil
}

func (s *MemoryStore) Nonce(_ context.Context, value string) (NonceRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	nonce, ok := s.nonces[value]
	if !ok {
		return NonceRecord{}, ErrNonceNotFound
	}
	return nonce, nil
}

func (s *MemoryStore) Idempotency(
	_ context.Context,
	playerID string,
	key string,
) (SubmitResponse, [32]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.idempotency[idempotencyKey(playerID, key)]
	return entry.Response, entry.RequestHash, ok, nil
}

func (s *MemoryStore) CommitSubmit(_ context.Context, input CommitRequest) (SubmitResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idemKey := idempotencyKey(input.PlayerID, input.IdempotencyKey)
	if existing, ok := s.idempotency[idemKey]; ok {
		if existing.RequestHash != input.RequestHash {
			return SubmitResponse{}, ErrIdempotencyConflict
		}
		return existing.Response, nil
	}

	nonce, ok := s.nonces[input.Nonce]
	if !ok ||
		nonce.PlayerID != input.PlayerID ||
		nonce.DeviceID != input.DeviceID ||
		nonce.AppBuild != input.AppBuild ||
		nonce.TraceID != input.TraceID ||
		nonce.EvidenceSchemaVersion != input.EvidenceSchemaVersion {
		return SubmitResponse{}, ErrNonceNotFound
	}
	if !input.Now.Before(nonce.ExpiresAt) {
		return SubmitResponse{}, ErrNonceNotFound
	}
	if nonce.UsedAt != nil {
		return SubmitResponse{}, ErrNonceUsed
	}
	if expiresAt, ok := s.replayHashes[input.ReplayHash]; ok {
		if input.Now.Before(expiresAt) {
			return SubmitResponse{}, ErrReplayDetected
		}
		delete(s.replayHashes, input.ReplayHash)
	}
	if last, ok := s.lastSubmission[input.PlayerID]; ok && input.Now.Sub(last) < time.Minute {
		return SubmitResponse{}, ErrSubmissionRate
	}
	dayStart := input.Now.UTC().Truncate(24 * time.Hour)
	count := 0
	for _, card := range s.cards {
		if card.OwnerID == input.PlayerID && !card.CreatedAt.Before(dayStart) {
			count++
		}
	}
	if count >= 10 {
		return SubmitResponse{}, ErrDailyLimit
	}

	usedAt := input.Now
	nonce.UsedAt = &usedAt
	s.nonces[input.Nonce] = nonce
	s.replayHashes[input.ReplayHash] = input.Now.Add(90 * 24 * time.Hour)
	s.idempotency[idemKey] = idempotencyEntry{
		Response:    input.Response,
		RequestHash: input.RequestHash,
	}
	s.cards = append(s.cards, input.Response.Card)
	s.lastSubmission[input.PlayerID] = input.Now
	return input.Response, nil
}

func (s *MemoryStore) CardCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.cards)
}

func deviceKey(playerID, deviceID string) string {
	return playerID + "\x00" + deviceID
}

func idempotencyKey(playerID, key string) string {
	return playerID + "\x00" + key
}
