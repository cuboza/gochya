ALTER TABLE pets
    ADD CONSTRAINT uq_pets_owner_id UNIQUE (owner_id, id);

CREATE TABLE player_loadouts (
    player_id      UUID PRIMARY KEY REFERENCES players(id) ON DELETE CASCADE,
    pet_id         UUID NOT NULL,
    card_ids       UUID[] NOT NULL,
    signature_idx  SMALLINT NOT NULL CHECK (signature_idx BETWEEN 0 AND 4),
    revision       BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    updated_at     TIMESTAMPTZ NOT NULL,
    FOREIGN KEY (player_id, pet_id)
        REFERENCES pets(owner_id, id),
    CHECK (
        array_ndims(card_ids) = 1
        AND array_lower(card_ids, 1) = 1
        AND array_upper(card_ids, 1) = 5
        AND array_position(card_ids, NULL) IS NULL
    ),
    CHECK (
        card_ids[1] <> ALL(card_ids[2:5])
        AND card_ids[2] <> ALL(card_ids[3:5])
        AND card_ids[3] <> ALL(card_ids[4:5])
        AND card_ids[4] <> card_ids[5]
    )
);

CREATE TABLE loadout_idempotency (
    player_id       UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    idempotency_key UUID NOT NULL,
    request_hash    BYTEA NOT NULL CHECK (octet_length(request_hash) = 32),
    response_body   JSONB NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL,
    expires_at      TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (player_id, idempotency_key),
    CHECK (expires_at > created_at)
);

CREATE INDEX idx_loadout_idempotency_expiry
    ON loadout_idempotency(expires_at);

-- player_loadouts is authoritative. The is_equipped/is_signature columns on
-- technique_cards are a transactionally maintained read projection only.
