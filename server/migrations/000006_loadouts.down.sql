UPDATE technique_cards
   SET is_equipped = FALSE,
       is_signature = FALSE
 WHERE is_equipped = TRUE OR is_signature = TRUE;

DROP TABLE IF EXISTS loadout_idempotency;
DROP TABLE IF EXISTS player_loadouts;

ALTER TABLE pets
    DROP CONSTRAINT IF EXISTS uq_pets_owner_id;
