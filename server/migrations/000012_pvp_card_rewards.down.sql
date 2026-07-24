DROP INDEX IF EXISTS idx_match_confirmations_player_card;

ALTER TABLE match_confirmations
    DROP CONSTRAINT IF EXISTS match_confirmations_card_pair,
    DROP COLUMN IF EXISTS card_seed,
    DROP COLUMN IF EXISTS card_id;
