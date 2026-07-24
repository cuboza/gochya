package shop

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

const (
	testPlayerID = "11111111-1111-4111-8111-111111111111"
	testKey      = "22222222-2222-4222-8222-222222222222"
)

func TestServiceCatalogIsReleaseScoped(t *testing.T) {
	service := shopService(t, &fakeStore{}, time.Time{})
	response, err := service.Catalog(context.Background(), testPlayerID)
	if err != nil {
		t.Fatalf("Catalog: %v", err)
	}
	expected := []CatalogItem{
		{ID: ItemApple, Category: CategoryCare, Currency: CurrencyKoins, UnitPrice: 20, IsStackable: true},
		{ID: ItemSteak, Category: CategoryCare, Currency: CurrencyKoins, UnitPrice: 80, IsStackable: true},
		{ID: ItemEnergyDrink, Category: CategoryCare, Currency: CurrencyKoins, UnitPrice: 50, IsStackable: true},
		{ID: ItemSoap, Category: CategoryCare, Currency: CurrencyKoins, UnitPrice: 30, IsStackable: true},
		{ID: ItemShampoo, Category: CategoryCare, Currency: CurrencyKoins, UnitPrice: 60, IsStackable: true},
		{ID: ItemLoveCrystal, Category: CategoryBreeding, Currency: CurrencyKoins, UnitPrice: 200, IsStackable: true},
	}
	if !reflect.DeepEqual(response.Items, expected) {
		t.Fatalf("catalog = %#v", response.Items)
	}
	response.Items[0].UnitPrice = 1
	repeated, err := service.Catalog(context.Background(), testPlayerID)
	if err != nil || repeated.Items[0].UnitPrice != 20 {
		t.Fatalf("catalog mutation leaked = %#v, %v", repeated, err)
	}
}

func TestServiceNormalizesPurchase(t *testing.T) {
	now := time.Date(2026, time.July, 24, 12, 30, 0, 0, time.UTC)
	store := &fakeStore{purchase: PurchaseResponse{
		ItemID:         ItemApple,
		ItemQuantity:   3,
		KoinsRemaining: 940,
	}}
	service := shopService(t, store, now)
	response, err := service.Purchase(
		context.Background(),
		testPlayerID,
		testKey,
		PurchaseRequest{ItemID: ItemApple, Quantity: 3},
	)
	if err != nil {
		t.Fatalf("Purchase: %v", err)
	}
	if !reflect.DeepEqual(response, store.purchase) {
		t.Fatalf("response = %#v", response)
	}
	if store.commit.PlayerID != testPlayerID ||
		store.commit.IdempotencyKey != testKey ||
		store.commit.RequestHash == ([32]byte{}) ||
		store.commit.Item.ID != ItemApple ||
		store.commit.Item.UnitPrice != 20 ||
		store.commit.Quantity != 3 ||
		!store.commit.Now.Equal(now) {
		t.Fatalf("commit = %#v", store.commit)
	}
}

func TestServiceRejectsInvalidPurchases(t *testing.T) {
	tests := []struct {
		name    string
		player  string
		key     string
		request PurchaseRequest
		code    string
	}{
		{"identity", "", testKey, PurchaseRequest{ItemID: ItemApple, Quantity: 1}, "identity_invalid"},
		{"key", testPlayerID, "bad", PurchaseRequest{ItemID: ItemApple, Quantity: 1}, "idempotency_key_invalid"},
		{"item", testPlayerID, testKey, PurchaseRequest{ItemID: "medicine", Quantity: 1}, "shop_item_invalid"},
		{"item whitespace", testPlayerID, testKey, PurchaseRequest{ItemID: " apple", Quantity: 1}, "shop_item_invalid"},
		{"zero quantity", testPlayerID, testKey, PurchaseRequest{ItemID: ItemApple}, "quantity_invalid"},
		{"large quantity", testPlayerID, testKey, PurchaseRequest{ItemID: ItemApple, Quantity: 101}, "quantity_invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := shopService(t, &fakeStore{}, time.Time{})
			_, err := service.Purchase(
				context.Background(),
				test.player,
				test.key,
				test.request,
			)
			var apiErr *Error
			if !errors.As(err, &apiErr) || apiErr.Code != test.code {
				t.Fatalf("error = %#v, want %q", err, test.code)
			}
		})
	}
}

func TestServiceMapsStoreAndInventoryResults(t *testing.T) {
	store := &fakeStore{
		err:       ErrInsufficientKoins,
		inventory: InventoryResponse{Koins: 7},
	}
	service := shopService(t, store, time.Time{})
	_, err := service.Purchase(
		context.Background(),
		testPlayerID,
		testKey,
		PurchaseRequest{ItemID: ItemApple, Quantity: 1},
	)
	var apiErr *Error
	if !errors.As(err, &apiErr) ||
		apiErr.Code != "insufficient_koins" ||
		apiErr.HTTPStatus != 409 {
		t.Fatalf("purchase error = %#v", err)
	}
	store.err = nil
	inventory, err := service.Inventory(context.Background(), testPlayerID)
	if err != nil || inventory.Koins != 7 || inventory.Items == nil ||
		len(inventory.Items) != 0 {
		t.Fatalf("inventory = %#v, %v", inventory, err)
	}
}

func shopService(t *testing.T, store Store, now time.Time) *Service {
	t.Helper()
	service, err := NewService(ServiceConfig{
		Store: store,
		Now:   func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}

type fakeStore struct {
	purchase  PurchaseResponse
	inventory InventoryResponse
	err       error
	commit    PurchaseCommit
}

func (s *fakeStore) Purchase(
	_ context.Context,
	input PurchaseCommit,
) (PurchaseResponse, error) {
	s.commit = input
	return s.purchase, s.err
}

func (s *fakeStore) Inventory(
	context.Context,
	string,
) (InventoryResponse, error) {
	return s.inventory, s.err
}
