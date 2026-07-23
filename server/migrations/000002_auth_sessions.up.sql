CREATE TABLE refresh_tokens (
    id                  UUID PRIMARY KEY,
    family_id           UUID NOT NULL,
    player_id           UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    device_id           TEXT,
    token_hash          BYTEA NOT NULL UNIQUE
                        CHECK (octet_length(token_hash) = 32),
    issued_at           TIMESTAMPTZ NOT NULL,
    expires_at          TIMESTAMPTZ NOT NULL,
    family_expires_at   TIMESTAMPTZ NOT NULL,
    revoked_at          TIMESTAMPTZ,
    replaced_by         UUID REFERENCES refresh_tokens(id),
    reuse_detected_at   TIMESTAMPTZ,
    CHECK (expires_at > issued_at),
    CHECK (family_expires_at >= expires_at)
);

CREATE INDEX idx_refresh_tokens_family
    ON refresh_tokens(family_id);
CREATE INDEX idx_refresh_tokens_player_expiry
    ON refresh_tokens(player_id, expires_at);

-- Refresh token plaintext is returned once and never persisted. Rotation locks
-- the current row, inserts its replacement and retires the old token in one
-- transaction. Reuse of a retired token revokes every row in the token family.
