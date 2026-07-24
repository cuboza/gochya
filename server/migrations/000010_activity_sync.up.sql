CREATE TABLE daily_activity (
    player_id          UUID NOT NULL
                       REFERENCES players(id) ON DELETE CASCADE,
    activity_date      DATE NOT NULL,
    pet_id             UUID NOT NULL
                       REFERENCES pets(id) ON DELETE CASCADE,
    snapshot           JSONB NOT NULL
                       CHECK (jsonb_typeof(snapshot) = 'object'),
    fingerprint        BYTEA NOT NULL
                       CHECK (octet_length(fingerprint) = 32),
    steps              BIGINT NOT NULL
                       CHECK (steps BETWEEN 0 AND 4294967295),
    sleep_minutes      INTEGER NOT NULL
                       CHECK (sleep_minutes BETWEEN 0 AND 65535),
    active_calories    INTEGER NOT NULL
                       CHECK (active_calories BETWEEN 0 AND 65535),
    goals              JSONB NOT NULL
                       CHECK (jsonb_typeof(goals) = 'object'),
    vitality_total     INTEGER NOT NULL
                       CHECK (vitality_total BETWEEN 0 AND 150),
    vitality_awarded   INTEGER NOT NULL DEFAULT 0
                       CHECK (vitality_awarded BETWEEN 0 AND 150),
    stat_gains         JSONB NOT NULL
                       CHECK (jsonb_typeof(stat_gains) = 'object'),
    stat_gains_applied JSONB NOT NULL
                       CHECK (jsonb_typeof(stat_gains_applied) = 'object'),
    source_metadata    TEXT NOT NULL
                       CHECK (length(source_metadata) BETWEEN 1 AND 128),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (player_id, activity_date)
);

CREATE INDEX idx_daily_activity_player_date
    ON daily_activity(player_id, activity_date DESC);
