CREATE TABLE matches (
    id                 UUID PRIMARY KEY,
    player_a           UUID NOT NULL REFERENCES players(id),
    player_b           UUID NOT NULL REFERENCES players(id),
    loadout_a          JSONB NOT NULL,
    loadout_b          JSONB NOT NULL,
    loadout_revision_a BIGINT NOT NULL CHECK (loadout_revision_a > 0),
    loadout_revision_b BIGINT NOT NULL CHECK (loadout_revision_b > 0),
    match_seed         BIGINT NOT NULL CHECK (match_seed >= 0),
    result             JSONB NOT NULL,
    mode               TEXT NOT NULL CHECK (mode = 'casual'),
    created_at         TIMESTAMPTZ NOT NULL,
    CHECK (player_a <> player_b)
);

CREATE INDEX idx_matches_player_a_created ON matches(player_a, created_at DESC);
CREATE INDEX idx_matches_player_b_created ON matches(player_b, created_at DESC);

CREATE TABLE match_idempotency (
    player_id       UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    idempotency_key UUID NOT NULL,
    request_hash    BYTEA NOT NULL CHECK (octet_length(request_hash) = 32),
    response_body   JSONB NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL,
    expires_at      TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (player_id, idempotency_key),
    CHECK (expires_at > created_at)
);

CREATE INDEX idx_match_idempotency_expiry ON match_idempotency(expires_at);
