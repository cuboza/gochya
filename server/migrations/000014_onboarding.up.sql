ALTER TABLE eggs
    ALTER COLUMN parent_a_id DROP NOT NULL,
    ALTER COLUMN parent_b_id DROP NOT NULL;

ALTER TABLE eggs
    ADD COLUMN origin TEXT NOT NULL DEFAULT 'breeding',
    ADD CONSTRAINT eggs_origin_check
        CHECK (origin IN ('breeding', 'starter')),
    ADD CONSTRAINT eggs_origin_parentage_check
        CHECK (
            (origin = 'breeding'
                AND parent_a_id IS NOT NULL
                AND parent_b_id IS NOT NULL)
            OR
            (origin = 'starter'
                AND parent_a_id IS NULL
                AND parent_b_id IS NULL
                AND mutated_genes = 0)
        );

CREATE UNIQUE INDEX uq_eggs_one_starter_per_owner
    ON eggs(owner_id)
    WHERE origin = 'starter';

CREATE TABLE onboarding_age_gate (
    player_id       UUID PRIMARY KEY REFERENCES players(id) ON DELETE CASCADE,
    age_band        TEXT NOT NULL CHECK (age_band IN ('under13', '13plus')),
    policy_version  TEXT NOT NULL DEFAULT 'coppa-v1'
                    CHECK (policy_version = 'coppa-v1'),
    idempotency_key UUID NOT NULL,
    recorded_at     TIMESTAMPTZ NOT NULL
);

CREATE TABLE onboarding_starter_selections (
    player_id       UUID PRIMARY KEY REFERENCES players(id) ON DELETE CASCADE,
    idempotency_key UUID NOT NULL,
    request_hash    BYTEA NOT NULL CHECK (octet_length(request_hash) = 32),
    egg_id          UUID NOT NULL UNIQUE REFERENCES eggs(id) ON DELETE RESTRICT,
    element         TEXT NOT NULL CHECK (element IN ('fire', 'water', 'earth')),
    response_body   JSONB NOT NULL CHECK (jsonb_typeof(response_body) = 'object'),
    created_at      TIMESTAMPTZ NOT NULL
);
