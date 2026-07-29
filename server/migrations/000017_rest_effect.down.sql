ALTER TABLE daily_activity
    DROP COLUMN IF EXISTS rest_applied;

ALTER TABLE pets
    DROP COLUMN IF EXISTS pending_rest_quality,
    DROP COLUMN IF EXISTS pending_rest_minutes;
