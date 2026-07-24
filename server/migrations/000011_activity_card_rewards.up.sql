CREATE TABLE activity_card_rewards (
    player_id     UUID NOT NULL,
    activity_date DATE NOT NULL,
    card_id       UUID NOT NULL UNIQUE
                  REFERENCES technique_cards(id) ON DELETE CASCADE,
    seed          BYTEA NOT NULL
                  CHECK (octet_length(seed) = 8),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (player_id, activity_date),
    FOREIGN KEY (player_id, activity_date)
        REFERENCES daily_activity(player_id, activity_date)
        ON DELETE CASCADE
);

CREATE INDEX idx_activity_card_rewards_player_created
    ON activity_card_rewards(player_id, created_at DESC);
