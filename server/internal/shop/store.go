package shop

import "context"

type Store interface {
	Purchase(context.Context, PurchaseCommit) (PurchaseResponse, error)
	Inventory(context.Context, string) (InventoryResponse, error)
}
