DROP TABLE IF EXISTS care_operations;

ALTER TABLE pets
    DROP COLUMN IF EXISTS sleeping_until,
    DROP COLUMN IF EXISTS mood_decay_remainder,
    DROP COLUMN IF EXISTS hygiene_decay_remainder,
    DROP COLUMN IF EXISTS energy_decay_remainder,
    DROP COLUMN IF EXISTS hunger_decay_remainder,
    DROP COLUMN IF EXISTS needs_updated_at,
    DROP COLUMN IF EXISTS care_revision;
