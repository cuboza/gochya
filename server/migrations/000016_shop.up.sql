CREATE TABLE shop_purchases (
    player_id          UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    idempotency_key    UUID NOT NULL,
    request_hash       BYTEA NOT NULL CHECK (octet_length(request_hash) = 32),
    item_id            TEXT NOT NULL CHECK (
        item_id IN (
            'apple',
            'steak',
            'energy_drink',
            'soap',
            'shampoo',
            'love_crystal'
        )
    ),
    quantity           INTEGER NOT NULL CHECK (quantity BETWEEN 1 AND 100),
    unit_price_koins   BIGINT NOT NULL CHECK (unit_price_koins > 0),
    total_price_koins  BIGINT NOT NULL CHECK (
        total_price_koins = unit_price_koins * quantity
    ),
    response_body      JSONB NOT NULL CHECK (jsonb_typeof(response_body) = 'object'),
    created_at         TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (player_id, idempotency_key)
);

CREATE INDEX idx_shop_purchases_player_created
    ON shop_purchases(player_id, created_at DESC, idempotency_key DESC);
