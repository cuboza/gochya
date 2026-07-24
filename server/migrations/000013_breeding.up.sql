CREATE TABLE eggs (
    id              UUID PRIMARY KEY,
    owner_id        UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    genome          JSONB NOT NULL CHECK (jsonb_typeof(genome) = 'object'),
    parent_a_id     UUID NOT NULL REFERENCES pets(id) ON DELETE RESTRICT,
    parent_b_id     UUID NOT NULL REFERENCES pets(id) ON DELETE RESTRICT,
    incubate_until  TIMESTAMPTZ NOT NULL,
    breeding_seed   BYTEA NOT NULL CHECK (octet_length(breeding_seed) = 8),
    mutated_genes   INTEGER NOT NULL DEFAULT 0
                    CHECK (mutated_genes BETWEEN 0 AND 16383),
    hatched_at      TIMESTAMPTZ,
    hatched_pet_id  UUID UNIQUE REFERENCES pets(id) ON DELETE RESTRICT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (parent_a_id <> parent_b_id),
    CHECK ((hatched_at IS NULL) = (hatched_pet_id IS NULL))
);

CREATE TABLE player_items (
    player_id UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    item_id   TEXT NOT NULL CHECK (item_id <> ''),
    quantity  INTEGER NOT NULL DEFAULT 0 CHECK (quantity >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (player_id, item_id)
);

CREATE TABLE item_transactions (
    id                  BIGSERIAL PRIMARY KEY,
    player_id           UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    item_id             TEXT NOT NULL,
    amount              INTEGER NOT NULL CHECK (amount <> 0),
    counterparty        TEXT NOT NULL,
    counterparty_amount INTEGER NOT NULL,
    reason              TEXT NOT NULL,
    ref_id              TEXT NOT NULL,
    idempotency_key     TEXT NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL,
    UNIQUE (player_id, idempotency_key, item_id),
    CHECK (amount + counterparty_amount = 0),
    FOREIGN KEY (player_id, item_id)
        REFERENCES player_items(player_id, item_id)
);

CREATE INDEX idx_item_transactions_player_created
    ON item_transactions(player_id, created_at DESC, id DESC);

CREATE TABLE breeding_idempotency (
    player_id      UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    idempotency_key UUID NOT NULL,
    request_hash   BYTEA NOT NULL CHECK (octet_length(request_hash) = 32),
    response_body  JSONB NOT NULL CHECK (jsonb_typeof(response_body) = 'object'),
    egg_id         UUID NOT NULL UNIQUE REFERENCES eggs(id) ON DELETE CASCADE,
    created_at     TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (player_id, idempotency_key)
);

CREATE INDEX idx_eggs_owner_incubating
    ON eggs(owner_id, created_at ASC, id ASC)
    WHERE hatched_at IS NULL;
