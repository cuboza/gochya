ALTER TABLE match_confirmations
    ADD COLUMN card_id UUID UNIQUE
        REFERENCES technique_cards(id) ON DELETE RESTRICT,
    ADD COLUMN card_seed BYTEA
        CHECK (card_seed IS NULL OR octet_length(card_seed) = 8),
    ADD CONSTRAINT match_confirmations_card_pair
        CHECK ((card_id IS NULL) = (card_seed IS NULL));

CREATE INDEX idx_match_confirmations_player_card
    ON match_confirmations(player_id, confirmed_at DESC)
    WHERE card_id IS NOT NULL;
