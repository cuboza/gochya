ALTER TABLE pets
    ADD COLUMN care_revision BIGINT NOT NULL DEFAULT 0
        CHECK (care_revision >= 0),
    ADD COLUMN needs_updated_at TIMESTAMPTZ,
    ADD COLUMN hunger_decay_remainder BIGINT NOT NULL DEFAULT 0
        CHECK (hunger_decay_remainder BETWEEN 0 AND 10799999),
    ADD COLUMN energy_decay_remainder BIGINT NOT NULL DEFAULT 0
        CHECK (energy_decay_remainder BETWEEN 0 AND 10799999),
    ADD COLUMN hygiene_decay_remainder BIGINT NOT NULL DEFAULT 0
        CHECK (hygiene_decay_remainder BETWEEN 0 AND 10799999),
    ADD COLUMN mood_decay_remainder BIGINT NOT NULL DEFAULT 0
        CHECK (mood_decay_remainder BETWEEN 0 AND 10799999),
    ADD COLUMN sleeping_until TIMESTAMPTZ;

UPDATE pets SET needs_updated_at = created_at WHERE needs_updated_at IS NULL;

ALTER TABLE pets
    ALTER COLUMN needs_updated_at SET DEFAULT NOW(),
    ALTER COLUMN needs_updated_at SET NOT NULL;

CREATE TABLE care_operations (
    player_id       UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    operation_id    UUID NOT NULL,
    pet_id          UUID NOT NULL REFERENCES pets(id) ON DELETE CASCADE,
    device_id       TEXT NOT NULL CHECK (device_id <> '' AND length(device_id) <= 128),
    request_hash    BYTEA NOT NULL CHECK (octet_length(request_hash) = 32),
    status          TEXT NOT NULL CHECK (
        status IN (
            'APPLIED',
            'REJECTED_PRECONDITION',
            'REJECTED_EXPIRED',
            'REJECTED_INVALID'
        )
    ),
    response_body   JSONB NOT NULL CHECK (jsonb_typeof(response_body) = 'object'),
    created_at      TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (player_id, operation_id)
);

CREATE INDEX idx_care_operations_player_created
    ON care_operations(player_id, created_at DESC, operation_id DESC);
