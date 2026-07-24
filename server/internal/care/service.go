package care

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gochya/gochya/server/internal/corebridge"
)

const (
	maxCommands      = 100
	maxDeviceIDBytes = 128
)

type ServiceConfig struct {
	Store Store
	Core  corebridge.CareEngine
	Now   func() time.Time
}

type Service struct {
	store Store
	core  corebridge.CareEngine
	now   func() time.Time
}

func NewService(config ServiceConfig) (*Service, error) {
	if config.Store == nil || config.Core == nil {
		return nil, errors.New("care store and core are required")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &Service{store: config.Store, core: config.Core, now: config.Now}, nil
}

func (s *Service) Sync(
	ctx context.Context,
	playerID string,
	ifMatch string,
	request SyncRequest,
) (SyncResponse, error) {
	if strings.TrimSpace(playerID) == "" {
		return SyncResponse{}, apiError(
			"identity_invalid",
			"authenticated player is required",
			http.StatusBadRequest,
		)
	}
	deviceID := strings.TrimSpace(request.DeviceID)
	if deviceID == "" || len(deviceID) > maxDeviceIDBytes || deviceID != request.DeviceID {
		return SyncResponse{}, apiError(
			"device_id_invalid",
			"deviceId must be a non-empty canonical identifier",
			http.StatusBadRequest,
		)
	}
	if len(request.Commands) == 0 || len(request.Commands) > maxCommands {
		return SyncResponse{}, apiError(
			"commands_invalid",
			"commands must contain between 1 and 100 entries",
			http.StatusBadRequest,
		)
	}
	headerRevision, err := strconv.ParseUint(ifMatch, 10, 64)
	if err != nil || ifMatch != strconv.FormatUint(headerRevision, 10) {
		return SyncResponse{}, apiError(
			"if_match_invalid",
			"If-Match must be a canonical unsigned revision",
			http.StatusBadRequest,
		)
	}
	if headerRevision != request.Commands[0].BaseRevision {
		return SyncResponse{}, apiError(
			"if_match_invalid",
			"If-Match must equal the first command baseRevision",
			http.StatusBadRequest,
		)
	}

	commands := make([]NormalizedCommand, len(request.Commands))
	var petID string
	for index, command := range request.Commands {
		normalized, err := normalizeCommand(command)
		if err != nil {
			return SyncResponse{}, err
		}
		if index == 0 {
			petID = normalized.PetID
		} else if normalized.PetID != petID {
			return SyncResponse{}, apiError(
				"aggregate_invalid",
				"one sync batch may target only one pet",
				http.StatusBadRequest,
			)
		}
		commands[index] = normalized
	}
	response, err := s.store.Reconcile(ctx, SyncCommit{
		PlayerID: playerID,
		DeviceID: deviceID,
		PetID:    petID,
		Commands: commands,
		Now:      s.now().UTC(),
	}, s.core)
	if err != nil {
		return SyncResponse{}, asAPIError(err)
	}
	return response, nil
}

func normalizeCommand(command CareCommand) (NormalizedCommand, error) {
	if !validUUID(command.OperationID) {
		return NormalizedCommand{}, apiError(
			"operation_id_invalid",
			"operationId must be a UUID",
			http.StatusBadRequest,
		)
	}
	if command.AggregateType != "pet" || !validUUID(command.AggregateID) {
		return NormalizedCommand{}, apiError(
			"aggregate_invalid",
			"aggregateType must be pet and aggregateId must be a UUID",
			http.StatusBadRequest,
		)
	}
	if command.SchemaVersion != SchemaVersionV1 {
		return NormalizedCommand{}, apiError(
			"schema_version_unsupported",
			"care command schemaVersion is not supported",
			http.StatusBadRequest,
		)
	}
	if command.ClientWallTime.IsZero() {
		return NormalizedCommand{}, apiError(
			"client_time_invalid",
			"clientWallTime must be an RFC3339 timestamp",
			http.StatusBadRequest,
		)
	}
	action, item, itemID, ok := careAction(command.OperationType, command.Arguments.ItemID)
	if !ok {
		return NormalizedCommand{}, apiError(
			"care_action_invalid",
			"operationType and itemId combination is not supported",
			http.StatusBadRequest,
		)
	}
	canonical, err := json.Marshal(struct {
		OperationID             string        `json:"operationId"`
		AggregateType           string        `json:"aggregateType"`
		AggregateID             string        `json:"aggregateId"`
		BaseRevision            uint64        `json:"baseRevision"`
		OperationType           string        `json:"operationType"`
		Arguments               CareArguments `json:"arguments"`
		ClientWallTime          time.Time     `json:"clientWallTime"`
		ClientMonotonicOffsetMS uint64        `json:"clientMonotonicOffsetMs"`
		SchemaVersion           uint16        `json:"schemaVersion"`
	}{
		OperationID:             strings.ToLower(command.OperationID),
		AggregateType:           command.AggregateType,
		AggregateID:             strings.ToLower(command.AggregateID),
		BaseRevision:            command.BaseRevision,
		OperationType:           command.OperationType,
		Arguments:               CareArguments{ItemID: itemID},
		ClientWallTime:          command.ClientWallTime.UTC(),
		ClientMonotonicOffsetMS: command.ClientMonotonicOffsetMS,
		SchemaVersion:           command.SchemaVersion,
	})
	if err != nil {
		return NormalizedCommand{}, asAPIError(err)
	}
	return NormalizedCommand{
		OperationID:    strings.ToLower(command.OperationID),
		PetID:          strings.ToLower(command.AggregateID),
		BaseRevision:   command.BaseRevision,
		OperationType:  command.OperationType,
		Action:         action,
		Item:           item,
		ItemID:         itemID,
		ClientWallTime: command.ClientWallTime.UTC(),
		RequestHash:    sha256.Sum256(canonical),
	}, nil
}

func careAction(operation string, itemID string) (uint8, uint8, string, bool) {
	switch operation {
	case OperationFeed:
		switch itemID {
		case ItemApple:
			return actionFeed, careItemApple, itemID, true
		case ItemSteak:
			return actionFeed, careItemSteak, itemID, true
		case ItemEnergyDrink:
			return actionFeed, careItemEnergyDrink, itemID, true
		default:
			return 0, 0, "", false
		}
	case OperationClean:
		switch itemID {
		case "":
			return actionClean, careItemNone, "", true
		case ItemSoap:
			return actionClean, careItemSoap, itemID, true
		case ItemShampoo:
			return actionClean, careItemShampoo, itemID, true
		default:
			return 0, 0, "", false
		}
	case OperationPlay:
		if itemID == "" {
			return actionPlay, careItemNone, "", true
		}
	case OperationSleep:
		if itemID == "" {
			return actionSleep, careItemNone, "", true
		}
	}
	return 0, 0, "", false
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' ||
		value[18] != '-' || value[23] != '-' {
		return false
	}
	decoded := make([]byte, 16)
	_, err := hex.Decode(decoded, []byte(strings.ReplaceAll(value, "-", "")))
	return err == nil
}
