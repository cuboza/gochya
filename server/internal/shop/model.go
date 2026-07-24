package shop

import "time"

const (
	CurrencyKoins = "koins"

	CategoryCare     = "care"
	CategoryBreeding = "breeding"

	ItemApple       = "apple"
	ItemSteak       = "steak"
	ItemEnergyDrink = "energy_drink"
	ItemSoap        = "soap"
	ItemShampoo     = "shampoo"
	ItemLoveCrystal = "love_crystal"
)

type CatalogItem struct {
	ID          string `json:"id"`
	Category    string `json:"category"`
	Currency    string `json:"currency"`
	UnitPrice   int64  `json:"unitPrice"`
	IsStackable bool   `json:"isStackable"`
}

type CatalogResponse struct {
	Items []CatalogItem `json:"items"`
}

type PurchaseRequest struct {
	ItemID   string `json:"itemId"`
	Quantity uint32 `json:"quantity"`
}

type PurchaseResponse struct {
	ItemID            string    `json:"itemId"`
	PurchasedQuantity uint32    `json:"purchasedQuantity"`
	ItemQuantity      uint32    `json:"itemQuantity"`
	UnitPriceKoins    int64     `json:"unitPriceKoins"`
	KoinsSpent        int64     `json:"koinsSpent"`
	KoinsRemaining    int64     `json:"koinsRemaining"`
	PurchasedAt       time.Time `json:"purchasedAt"`
}

type OwnedItem struct {
	ItemID   string `json:"itemId"`
	Quantity uint32 `json:"quantity"`
}

type InventoryResponse struct {
	Koins int64       `json:"koins"`
	Items []OwnedItem `json:"items"`
}

type PurchaseCommit struct {
	PlayerID       string
	IdempotencyKey string
	RequestHash    [32]byte
	Item           CatalogItem
	Quantity       uint32
	Now            time.Time
}
