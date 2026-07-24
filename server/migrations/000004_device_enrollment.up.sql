CREATE TABLE device_enrollment_challenges (
    challenge_hash BYTEA PRIMARY KEY CHECK (octet_length(challenge_hash) = 32),
    player_id      UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    device_id      TEXT NOT NULL,
    platform       TEXT NOT NULL CHECK (platform IN ('wear_os', 'watch_os')),
    app_build      TEXT NOT NULL,
    issued_at      TIMESTAMPTZ NOT NULL,
    expires_at     TIMESTAMPTZ NOT NULL,
    used_at        TIMESTAMPTZ,
    CHECK (expires_at > issued_at)
);

CREATE INDEX idx_device_enrollment_challenges_expiry
    ON device_enrollment_challenges(expires_at);
