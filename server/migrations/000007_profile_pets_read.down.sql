DROP INDEX IF EXISTS idx_pets_owner_active_created;

ALTER TABLE pets
    DROP CONSTRAINT IF EXISTS ck_pets_stats_contract,
    DROP CONSTRAINT IF EXISTS ck_pets_needs_contract;
