package care

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gochya/gochya/server/internal/corebridge"
)

const (
	testPlayerID = "11111111-1111-4111-8111-111111111111"
	testPetID    = "22222222-2222-4222-8222-222222222222"
	testOpID     = "33333333-3333-4333-8333-333333333333"
)

func TestServiceNormalizesCareBatch(t *testing.T) {
	now := time.Date(2026, time.July, 24, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{
		response: SyncResponse{
			NewRevision: 2,
			ServerTime:  now,
		},
	}
	service := careService(t, store, now)
	request := validSyncRequest(now)
	request.Commands = append(request.Commands, CareCommand{
		OperationID:             "44444444-4444-4444-8444-444444444444",
		AggregateType:           "pet",
		AggregateID:             strings.ToUpper(testPetID),
		BaseRevision:            1,
		OperationType:           OperationClean,
		Arguments:               CareArguments{},
		ClientWallTime:          now.Add(-time.Minute),
		ClientMonotonicOffsetMS: 2_000,
		SchemaVersion:           SchemaVersionV1,
	})
	response, err := service.Sync(
		context.Background(),
		testPlayerID,
		"0",
		request,
	)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if !reflect.DeepEqual(response, store.response) {
		t.Fatalf("response = %#v", response)
	}
	if store.commit.PlayerID != testPlayerID ||
		store.commit.DeviceID != "phone-1" ||
		store.commit.PetID != testPetID ||
		store.commit.Now != now ||
		len(store.commit.Commands) != 2 {
		t.Fatalf("commit = %#v", store.commit)
	}
	first := store.commit.Commands[0]
	if first.OperationID != testOpID ||
		first.PetID != testPetID ||
		first.Action != actionFeed ||
		first.Item != careItemApple ||
		first.ItemID != ItemApple ||
		first.RequestHash == ([32]byte{}) {
		t.Fatalf("first command = %#v", first)
	}
	second := store.commit.Commands[1]
	if second.Action != actionClean ||
		second.Item != careItemNone ||
		second.BaseRevision != 1 {
		t.Fatalf("second command = %#v", second)
	}
}

func TestServiceRejectsInvalidCareBatches(t *testing.T) {
	now := time.Date(2026, time.July, 24, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		player  string
		ifMatch string
		mutate  func(*SyncRequest)
		code    string
	}{
		{"identity", "", "0", nil, "identity_invalid"},
		{"device", testPlayerID, "0", func(r *SyncRequest) { r.DeviceID = " phone " }, "device_id_invalid"},
		{"empty commands", testPlayerID, "0", func(r *SyncRequest) { r.Commands = nil }, "commands_invalid"},
		{"if match", testPlayerID, "00", nil, "if_match_invalid"},
		{"if match mismatch", testPlayerID, "1", nil, "if_match_invalid"},
		{"operation ID", testPlayerID, "0", func(r *SyncRequest) { r.Commands[0].OperationID = "bad" }, "operation_id_invalid"},
		{"aggregate type", testPlayerID, "0", func(r *SyncRequest) { r.Commands[0].AggregateType = "player" }, "aggregate_invalid"},
		{"aggregate ID", testPlayerID, "0", func(r *SyncRequest) { r.Commands[0].AggregateID = "bad" }, "aggregate_invalid"},
		{"schema", testPlayerID, "0", func(r *SyncRequest) { r.Commands[0].SchemaVersion = 2 }, "schema_version_unsupported"},
		{"time", testPlayerID, "0", func(r *SyncRequest) { r.Commands[0].ClientWallTime = time.Time{} }, "client_time_invalid"},
		{"action", testPlayerID, "0", func(r *SyncRequest) { r.Commands[0].OperationType = "hug" }, "care_action_invalid"},
		{"item combination", testPlayerID, "0", func(r *SyncRequest) { r.Commands[0].Arguments.ItemID = ItemSoap }, "care_action_invalid"},
		{
			"multiple pets",
			testPlayerID,
			"0",
			func(r *SyncRequest) {
				second := r.Commands[0]
				second.OperationID = "44444444-4444-4444-8444-444444444444"
				second.AggregateID = "55555555-5555-4555-8555-555555555555"
				r.Commands = append(r.Commands, second)
			},
			"aggregate_invalid",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := validSyncRequest(now)
			if test.mutate != nil {
				test.mutate(&request)
			}
			service := careService(t, &fakeStore{}, now)
			_, err := service.Sync(
				context.Background(),
				test.player,
				test.ifMatch,
				request,
			)
			var apiErr *Error
			if !errors.As(err, &apiErr) || apiErr.Code != test.code {
				t.Fatalf("error = %#v, want code %q", err, test.code)
			}
		})
	}
}

func TestServiceMapsStoreErrors(t *testing.T) {
	now := time.Date(2026, time.July, 24, 12, 0, 0, 0, time.UTC)
	service := careService(t, &fakeStore{err: ErrPetNotFound}, now)
	_, err := service.Sync(
		context.Background(),
		testPlayerID,
		"0",
		validSyncRequest(now),
	)
	var apiErr *Error
	if !errors.As(err, &apiErr) ||
		apiErr.Code != "pet_not_found" ||
		apiErr.HTTPStatus != 404 {
		t.Fatalf("error = %#v", err)
	}
}

func validSyncRequest(now time.Time) SyncRequest {
	return SyncRequest{
		DeviceID: "phone-1",
		Commands: []CareCommand{{
			OperationID:             testOpID,
			AggregateType:           "pet",
			AggregateID:             testPetID,
			BaseRevision:            0,
			OperationType:           OperationFeed,
			Arguments:               CareArguments{ItemID: ItemApple},
			ClientWallTime:          now.Add(-2 * time.Minute),
			ClientMonotonicOffsetMS: 1_000,
			SchemaVersion:           SchemaVersionV1,
		}},
	}
}

func careService(t *testing.T, store Store, now time.Time) *Service {
	t.Helper()
	service, err := NewService(ServiceConfig{
		Store: store,
		Core:  fakeCore{},
		Now:   func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}

type fakeStore struct {
	response SyncResponse
	err      error
	commit   SyncCommit
}

func (s *fakeStore) Reconcile(
	_ context.Context,
	input SyncCommit,
	_ corebridge.CareEngine,
) (SyncResponse, error) {
	s.commit = input
	return s.response, s.err
}

type fakeCore struct{}

func (fakeCore) AdvanceNeeds(
	_ context.Context,
	state corebridge.NeedsState,
	_ uint64,
) (corebridge.NeedsState, error) {
	return state, nil
}

func (fakeCore) ApplyCare(
	_ context.Context,
	state corebridge.NeedsState,
	_ uint8,
	_ uint8,
) (corebridge.NeedsState, error) {
	return state, nil
}
