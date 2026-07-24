package shop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

const maxPurchaseQuantity = 100

var releaseCatalog = [...]CatalogItem{
	{ID: ItemApple, Category: CategoryCare, Currency: CurrencyKoins, UnitPrice: 20, IsStackable: true},
	{ID: ItemSteak, Category: CategoryCare, Currency: CurrencyKoins, UnitPrice: 80, IsStackable: true},
	{ID: ItemEnergyDrink, Category: CategoryCare, Currency: CurrencyKoins, UnitPrice: 50, IsStackable: true},
	{ID: ItemSoap, Category: CategoryCare, Currency: CurrencyKoins, UnitPrice: 30, IsStackable: true},
	{ID: ItemShampoo, Category: CategoryCare, Currency: CurrencyKoins, UnitPrice: 60, IsStackable: true},
	{ID: ItemLoveCrystal, Category: CategoryBreeding, Currency: CurrencyKoins, UnitPrice: 200, IsStackable: true},
}

type ServiceConfig struct {
	Store Store
	Now   func() time.Time
}

type Service struct {
	store Store
	now   func() time.Time
}

func NewService(config ServiceConfig) (*Service, error) {
	if config.Store == nil {
		return nil, errors.New("shop store is required")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &Service{store: config.Store, now: config.Now}, nil
}

func (s *Service) Catalog(_ context.Context, playerID string) (CatalogResponse, error) {
	if err := validateIdentity(playerID); err != nil {
		return CatalogResponse{}, err
	}
	items := make([]CatalogItem, len(releaseCatalog))
	copy(items, releaseCatalog[:])
	return CatalogResponse{Items: items}, nil
}

func (s *Service) Purchase(
	ctx context.Context,
	playerID string,
	idempotencyKey string,
	request PurchaseRequest,
) (PurchaseResponse, error) {
	if err := validateIdentity(playerID); err != nil {
		return PurchaseResponse{}, err
	}
	if !validUUID(idempotencyKey) {
		return PurchaseResponse{}, apiError(
			"idempotency_key_invalid",
			"Idempotency-Key must be a UUID",
			http.StatusBadRequest,
		)
	}
	if request.ItemID == "" || request.ItemID != strings.TrimSpace(request.ItemID) {
		return PurchaseResponse{}, apiError(
			"shop_item_invalid",
			"itemId is not available in the release catalog",
			http.StatusBadRequest,
		)
	}
	item, ok := catalogItem(request.ItemID)
	if !ok {
		return PurchaseResponse{}, apiError(
			"shop_item_invalid",
			"itemId is not available in the release catalog",
			http.StatusBadRequest,
		)
	}
	if request.Quantity == 0 || request.Quantity > maxPurchaseQuantity {
		return PurchaseResponse{}, apiError(
			"quantity_invalid",
			"quantity must be between 1 and 100",
			http.StatusBadRequest,
		)
	}
	canonical, err := json.Marshal(struct {
		ItemID   string `json:"itemId"`
		Quantity uint32 `json:"quantity"`
	}{
		ItemID:   item.ID,
		Quantity: request.Quantity,
	})
	if err != nil {
		return PurchaseResponse{}, asAPIError(err)
	}
	response, err := s.store.Purchase(ctx, PurchaseCommit{
		PlayerID:       playerID,
		IdempotencyKey: strings.ToLower(idempotencyKey),
		RequestHash:    sha256.Sum256(canonical),
		Item:           item,
		Quantity:       request.Quantity,
		Now:            s.now().UTC(),
	})
	if err != nil {
		return PurchaseResponse{}, asAPIError(err)
	}
	return response, nil
}

func (s *Service) Inventory(
	ctx context.Context,
	playerID string,
) (InventoryResponse, error) {
	if err := validateIdentity(playerID); err != nil {
		return InventoryResponse{}, err
	}
	response, err := s.store.Inventory(ctx, playerID)
	if err != nil {
		return InventoryResponse{}, asAPIError(err)
	}
	if response.Items == nil {
		response.Items = []OwnedItem{}
	}
	return response, nil
}

func validateIdentity(playerID string) error {
	if strings.TrimSpace(playerID) == "" {
		return apiError(
			"identity_invalid",
			"authenticated player is required",
			http.StatusBadRequest,
		)
	}
	return nil
}

func catalogItem(itemID string) (CatalogItem, bool) {
	for _, item := range releaseCatalog {
		if item.ID == itemID {
			return item, true
		}
	}
	return CatalogItem{}, false
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
