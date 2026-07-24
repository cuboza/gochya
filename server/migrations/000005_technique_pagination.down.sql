DROP INDEX IF EXISTS idx_technique_cards_owner_created;

CREATE INDEX idx_technique_cards_owner_created
    ON technique_cards(owner_id, created_at DESC);
