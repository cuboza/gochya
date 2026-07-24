CREATE TABLE player_wallet (
    player_id      UUID PRIMARY KEY
                   REFERENCES players(id) ON DELETE CASCADE,
    koins          BIGINT NOT NULL DEFAULT 0 CHECK (koins >= 0),
    vitality_daily INTEGER NOT NULL DEFAULT 0
                   CHECK (vitality_daily BETWEEN 0 AND 150),
    vitality_date  DATE NOT NULL DEFAULT ((CURRENT_TIMESTAMP AT TIME ZONE 'UTC')::DATE),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE transactions (
    id                  BIGSERIAL PRIMARY KEY,
    player_id           UUID NOT NULL
                        REFERENCES players(id) ON DELETE CASCADE,
    currency            TEXT NOT NULL
                        CHECK (currency IN ('koins', 'vitality')),
    amount              BIGINT NOT NULL CHECK (amount <> 0),
    counterparty        TEXT NOT NULL,
    counterparty_amount BIGINT NOT NULL,
    reason              TEXT NOT NULL,
    ref_id              TEXT NOT NULL,
    idempotency_key     TEXT NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL,
    UNIQUE (player_id, idempotency_key),
    CHECK (amount + counterparty_amount = 0)
);

CREATE INDEX idx_transactions_player_currency
    ON transactions(player_id, currency, created_at DESC);

CREATE TABLE match_confirmations (
    match_id      UUID NOT NULL REFERENCES matches(id) ON DELETE CASCADE,
    player_id     UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    outcome       TEXT NOT NULL CHECK (outcome IN ('win', 'loss', 'draw')),
    koins_awarded INTEGER NOT NULL CHECK (koins_awarded BETWEEN 0 AND 30),
    confirmed_at  TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (match_id, player_id)
);

CREATE INDEX idx_match_confirmations_player_created
    ON match_confirmations(player_id, confirmed_at DESC, match_id DESC);
