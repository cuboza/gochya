CREATE TABLE players (
    id              UUID PRIMARY KEY,
    username        TEXT UNIQUE NOT NULL,
    display_name    TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen       TIMESTAMPTZ,
    auth_method     TEXT NOT NULL
                    CHECK (auth_method IN ('apple', 'samsung', 'google')),
    auth_subject    TEXT NOT NULL,
    timezone        TEXT,
    streak_days     INTEGER NOT NULL DEFAULT 0 CHECK (streak_days >= 0),
    streak_last_day DATE,
    UNIQUE (auth_method, auth_subject)
);

CREATE TABLE pets (
    id               UUID PRIMARY KEY,
    owner_id         UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    genome           JSONB NOT NULL CHECK (jsonb_typeof(genome) = 'object'),
    name             TEXT,
    stage            TEXT NOT NULL
                     CHECK (stage IN ('egg', 'baby', 'teen', 'adult', 'premium')),
    level            INTEGER NOT NULL DEFAULT 1 CHECK (level >= 1),
    xp               BIGINT NOT NULL DEFAULT 0 CHECK (xp >= 0),
    needs            JSONB NOT NULL CHECK (jsonb_typeof(needs) = 'object'),
    stats            JSONB NOT NULL CHECK (jsonb_typeof(stats) = 'object'),
    generation       INTEGER NOT NULL DEFAULT 0 CHECK (generation >= 0),
    is_active        BOOLEAN NOT NULL DEFAULT FALSE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    parent_a_id      UUID REFERENCES pets(id),
    parent_b_id      UUID REFERENCES pets(id),
    last_bred_at     TIMESTAMPTZ,
    needs_zero_since TIMESTAMPTZ,
    is_weak          BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX idx_pets_owner ON pets(owner_id);
CREATE UNIQUE INDEX idx_pets_one_active_per_owner
    ON pets(owner_id)
    WHERE is_active;

CREATE TABLE technique_cards (
    id           UUID PRIMARY KEY,
    owner_id     UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    card_data    JSONB NOT NULL CHECK (jsonb_typeof(card_data) = 'object'),
    is_equipped  BOOLEAN NOT NULL DEFAULT FALSE,
    is_signature BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_technique_cards_owner_created
    ON technique_cards(owner_id, created_at DESC);

-- This is the smallest authoritative schema required by the implemented
-- auth/device/Dojo vertical slice. Later feature migrations add economy,
-- inventory, breeding and PvP tables instead of making the API depend on
-- manually provisioned relations.
