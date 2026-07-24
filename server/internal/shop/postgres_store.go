package shop

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) (*PostgresStore, error) {
	if pool == nil {
		return nil, errors.New("PostgreSQL pool is required")
	}
	return &PostgresStore{pool: pool}, nil
}

func (s *PostgresStore) Purchase(
	ctx context.Context,
	input PurchaseCommit,
) (PurchaseResponse, error) {
	now := input.Now.UTC().Truncate(time.Microsecond)
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return PurchaseResponse{}, fmt.Errorf("begin shop purchase: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockPlayer(ctx, tx, input.PlayerID); err != nil {
		return PurchaseResponse{}, err
	}
	response, found, conflict, err := existingPurchase(ctx, tx, input)
	if err != nil {
		return PurchaseResponse{}, err
	}
	if conflict {
		return PurchaseResponse{}, ErrIdempotencyConflict
	}
	if found {
		if err := tx.Commit(ctx); err != nil {
			return PurchaseResponse{}, fmt.Errorf("commit repeated shop purchase: %w", err)
		}
		return response, nil
	}

	total := input.Item.UnitPrice * int64(input.Quantity)
	koins, err := lockWallet(ctx, tx, input.PlayerID)
	if err != nil {
		return PurchaseResponse{}, err
	}
	if koins < total {
		return PurchaseResponse{}, ErrInsufficientKoins
	}
	remaining := koins - total
	if err := deductKoins(ctx, tx, input, total, now); err != nil {
		return PurchaseResponse{}, err
	}
	itemQuantity, err := addItem(ctx, tx, input, now)
	if err != nil {
		return PurchaseResponse{}, err
	}
	if err := recordItemCredit(ctx, tx, input, now); err != nil {
		return PurchaseResponse{}, err
	}
	response = PurchaseResponse{
		ItemID:            input.Item.ID,
		PurchasedQuantity: input.Quantity,
		ItemQuantity:      itemQuantity,
		UnitPriceKoins:    input.Item.UnitPrice,
		KoinsSpent:        total,
		KoinsRemaining:    remaining,
		PurchasedAt:       now,
	}
	if err := recordPurchase(ctx, tx, input, response, total, now); err != nil {
		return PurchaseResponse{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PurchaseResponse{}, fmt.Errorf("commit shop purchase: %w", err)
	}
	return response, nil
}

func (s *PostgresStore) Inventory(
	ctx context.Context,
	playerID string,
) (InventoryResponse, error) {
	var exists bool
	var koins int64
	if err := s.pool.QueryRow(ctx, `SELECT
		EXISTS(SELECT 1 FROM players WHERE id=$1),
		COALESCE((SELECT koins FROM player_wallet WHERE player_id=$1),0)`,
		playerID,
	).Scan(&exists, &koins); err != nil {
		return InventoryResponse{}, fmt.Errorf("query item inventory owner: %w", err)
	}
	if !exists {
		return InventoryResponse{}, ErrPlayerNotFound
	}
	rows, err := s.pool.Query(ctx, `SELECT item_id,quantity
		FROM player_items
		WHERE player_id=$1 AND quantity>0
		ORDER BY item_id ASC`,
		playerID,
	)
	if err != nil {
		return InventoryResponse{}, fmt.Errorf("query item inventory: %w", err)
	}
	defer rows.Close()
	items := make([]OwnedItem, 0)
	for rows.Next() {
		var item OwnedItem
		var quantity int64
		if err := rows.Scan(&item.ItemID, &quantity); err != nil {
			return InventoryResponse{}, fmt.Errorf("scan item inventory: %w", err)
		}
		if item.ItemID == "" || quantity <= 0 || quantity > math.MaxUint32 {
			return InventoryResponse{}, ErrStoredResponse
		}
		item.Quantity = uint32(quantity)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return InventoryResponse{}, fmt.Errorf("iterate item inventory: %w", err)
	}
	return InventoryResponse{Koins: koins, Items: items}, nil
}

func lockPlayer(ctx context.Context, tx pgx.Tx, playerID string) error {
	var lock int
	err := tx.QueryRow(
		ctx,
		`SELECT 1 FROM players WHERE id=$1 FOR UPDATE`,
		playerID,
	).Scan(&lock)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPlayerNotFound
	}
	if err != nil {
		return fmt.Errorf("lock shop player: %w", err)
	}
	return nil
}

func existingPurchase(
	ctx context.Context,
	tx pgx.Tx,
	input PurchaseCommit,
) (PurchaseResponse, bool, bool, error) {
	var storedHash, responseJSON []byte
	var storedItemID string
	var storedQuantity uint32
	var storedUnitPrice, storedTotal int64
	err := tx.QueryRow(ctx, `SELECT
		request_hash,item_id,quantity,unit_price_koins,total_price_koins,response_body
		FROM shop_purchases
		WHERE player_id=$1 AND idempotency_key=$2
		FOR UPDATE`,
		input.PlayerID,
		input.IdempotencyKey,
	).Scan(
		&storedHash,
		&storedItemID,
		&storedQuantity,
		&storedUnitPrice,
		&storedTotal,
		&responseJSON,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return PurchaseResponse{}, false, false, nil
	}
	if err != nil {
		return PurchaseResponse{}, false, false, fmt.Errorf("query shop purchase: %w", err)
	}
	if len(storedHash) != len(input.RequestHash) ||
		!bytes.Equal(storedHash, input.RequestHash[:]) {
		return PurchaseResponse{}, false, true, nil
	}
	var response PurchaseResponse
	if err := json.Unmarshal(responseJSON, &response); err != nil ||
		storedItemID != input.Item.ID ||
		storedQuantity != input.Quantity ||
		storedUnitPrice <= 0 ||
		storedTotal != storedUnitPrice*int64(storedQuantity) ||
		response.ItemID != storedItemID ||
		response.PurchasedQuantity != storedQuantity ||
		response.UnitPriceKoins != storedUnitPrice ||
		response.KoinsSpent != storedTotal ||
		response.ItemQuantity < response.PurchasedQuantity ||
		response.KoinsRemaining < 0 ||
		response.PurchasedAt.IsZero() {
		return PurchaseResponse{}, false, false, ErrStoredResponse
	}
	response.PurchasedAt = response.PurchasedAt.UTC()
	return response, true, false, nil
}

func lockWallet(ctx context.Context, tx pgx.Tx, playerID string) (int64, error) {
	var koins int64
	err := tx.QueryRow(ctx, `SELECT koins FROM player_wallet
		WHERE player_id=$1 FOR UPDATE`,
		playerID,
	).Scan(&koins)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("lock shop wallet: %w", err)
	}
	return koins, nil
}

func deductKoins(
	ctx context.Context,
	tx pgx.Tx,
	input PurchaseCommit,
	total int64,
	now time.Time,
) error {
	command, err := tx.Exec(ctx, `UPDATE player_wallet
		SET koins=koins-$2,updated_at=$3
		WHERE player_id=$1 AND koins>=$2`,
		input.PlayerID,
		total,
		now,
	)
	if err != nil {
		return fmt.Errorf("deduct shop Koins: %w", err)
	}
	if command.RowsAffected() != 1 {
		return ErrInsufficientKoins
	}
	if _, err := tx.Exec(ctx, `INSERT INTO transactions(
		player_id,currency,amount,counterparty,counterparty_amount,
		reason,ref_id,idempotency_key,created_at)
		VALUES($1,'koins',-$2::BIGINT,'system:shop',$2::BIGINT,
		'shop_purchase',$3,'shop:' || $4,$5)`,
		input.PlayerID,
		total,
		input.Item.ID,
		input.IdempotencyKey,
		now,
	); err != nil {
		return fmt.Errorf("insert shop Koins ledger entry: %w", err)
	}
	return nil
}

func addItem(
	ctx context.Context,
	tx pgx.Tx,
	input PurchaseCommit,
	now time.Time,
) (uint32, error) {
	var quantity int64
	err := tx.QueryRow(ctx, `INSERT INTO player_items(
		player_id,item_id,quantity,updated_at)
		VALUES($1,$2,$3,$4)
		ON CONFLICT(player_id,item_id) DO UPDATE
		SET quantity=player_items.quantity+EXCLUDED.quantity,
		    updated_at=EXCLUDED.updated_at
		WHERE player_items.quantity <= 2147483647-EXCLUDED.quantity
		RETURNING quantity`,
		input.PlayerID,
		input.Item.ID,
		input.Quantity,
		now,
	).Scan(&quantity)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrInventoryOverflow
	}
	if err != nil {
		return 0, fmt.Errorf("credit purchased item: %w", err)
	}
	if quantity <= 0 || quantity > math.MaxUint32 {
		return 0, ErrInventoryOverflow
	}
	return uint32(quantity), nil
}

func recordItemCredit(
	ctx context.Context,
	tx pgx.Tx,
	input PurchaseCommit,
	now time.Time,
) error {
	if _, err := tx.Exec(ctx, `INSERT INTO item_transactions(
		player_id,item_id,amount,counterparty,counterparty_amount,
		reason,ref_id,idempotency_key,created_at)
		VALUES($1,$2,$3,'system:shop',-$3::INTEGER,
		'shop_purchase',$4,'shop:' || $4,$5)`,
		input.PlayerID,
		input.Item.ID,
		input.Quantity,
		input.IdempotencyKey,
		now,
	); err != nil {
		return fmt.Errorf("insert shop item ledger entry: %w", err)
	}
	return nil
}

func recordPurchase(
	ctx context.Context,
	tx pgx.Tx,
	input PurchaseCommit,
	response PurchaseResponse,
	total int64,
	now time.Time,
) error {
	responseJSON, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("encode shop purchase response: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO shop_purchases(
		player_id,idempotency_key,request_hash,item_id,quantity,
		unit_price_koins,total_price_koins,response_body,created_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		input.PlayerID,
		input.IdempotencyKey,
		input.RequestHash[:],
		input.Item.ID,
		input.Quantity,
		input.Item.UnitPrice,
		total,
		responseJSON,
		now,
	); err != nil {
		return fmt.Errorf("insert shop purchase: %w", err)
	}
	return nil
}
