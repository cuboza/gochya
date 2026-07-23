-- The players and pets tables are created by the base schema described in
-- docs/03-architecture/BACKEND.md.

CREATE TABLE registered_devices (
    player_id       UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    device_id       TEXT NOT NULL,
    public_key      BYTEA NOT NULL CHECK (octet_length(public_key) = 32),
    platform        TEXT NOT NULL CHECK (platform IN ('wear_os', 'watch_os')),
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    registered_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    disabled_at     TIMESTAMPTZ,
    PRIMARY KEY (player_id, device_id)
);

CREATE TABLE dojo_nonces (
    nonce_hash              BYTEA PRIMARY KEY CHECK (octet_length(nonce_hash) = 32),
    player_id               UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    device_id               TEXT NOT NULL,
    app_build               TEXT NOT NULL,
    challenge               TEXT NOT NULL,
    evidence_schema_version SMALLINT NOT NULL CHECK (evidence_schema_version > 0),
    issued_at               TIMESTAMPTZ NOT NULL,
    expires_at              TIMESTAMPTZ NOT NULL,
    used_at                 TIMESTAMPTZ,
    CHECK (expires_at > issued_at),
    FOREIGN KEY (player_id, device_id)
        REFERENCES registered_devices(player_id, device_id)
);
CREATE INDEX idx_dojo_nonces_expiry ON dojo_nonces(expires_at);

CREATE TABLE dojo_replay_hashes (
    replay_hash     BYTEA PRIMARY KEY CHECK (octet_length(replay_hash) = 32),
    player_id       UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    card_id         UUID NOT NULL REFERENCES technique_cards(id) ON DELETE CASCADE,
    created_at      TIMESTAMPTZ NOT NULL,
    expires_at      TIMESTAMPTZ NOT NULL,
    CHECK (expires_at > created_at)
);
CREATE INDEX idx_dojo_replay_expiry ON dojo_replay_hashes(expires_at);

CREATE TABLE idempotency_results (
    player_id       UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    idempotency_key UUID NOT NULL,
    request_hash    BYTEA NOT NULL CHECK (octet_length(request_hash) = 32),
    http_status     SMALLINT NOT NULL CHECK (http_status BETWEEN 200 AND 599),
    response_body   JSONB NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL,
    expires_at      TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (player_id, idempotency_key),
    CHECK (expires_at > created_at)
);
CREATE INDEX idx_idempotency_results_expiry ON idempotency_results(expires_at);

CREATE TABLE dojo_submission_audit (
    card_id             UUID PRIMARY KEY REFERENCES technique_cards(id) ON DELETE CASCADE,
    player_id           UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    device_id           TEXT NOT NULL,
    evidence_verdict    TEXT NOT NULL CHECK (evidence_verdict IN ('VALID', 'SUSPECT')),
    heart_fail_reason   SMALLINT NOT NULL DEFAULT 0,
    app_build           TEXT NOT NULL,
    classifier_version  TEXT NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL,
    FOREIGN KEY (player_id, device_id)
        REFERENCES registered_devices(player_id, device_id)
);
CREATE INDEX idx_dojo_submission_daily
    ON dojo_submission_audit(player_id, created_at);

-- PostgresStore.CommitSubmit locks the player and nonce rows, checks the
-- minute/day limits and replay uniqueness, then inserts the card, replay hash,
-- audit row and idempotency result before marking the nonce used.
