DROP TABLE IF EXISTS onboarding_starter_selections;
DROP TABLE IF EXISTS onboarding_age_gate;
DROP INDEX IF EXISTS uq_eggs_one_starter_per_owner;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM eggs WHERE origin = 'starter') THEN
        RAISE EXCEPTION
            'cannot downgrade onboarding migration while starter eggs exist';
    END IF;
END
$$;

ALTER TABLE eggs
    DROP CONSTRAINT IF EXISTS eggs_origin_parentage_check,
    DROP CONSTRAINT IF EXISTS eggs_origin_check,
    DROP COLUMN IF EXISTS origin,
    ALTER COLUMN parent_a_id SET NOT NULL,
    ALTER COLUMN parent_b_id SET NOT NULL;
