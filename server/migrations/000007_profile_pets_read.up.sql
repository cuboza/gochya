ALTER TABLE pets
    ADD CONSTRAINT ck_pets_needs_contract CHECK (
        needs ?& ARRAY['hunger', 'energy', 'hygiene', 'mood']::TEXT[]
        AND needs - ARRAY['hunger', 'energy', 'hygiene', 'mood']::TEXT[]
            = '{}'::JSONB
        AND jsonb_typeof(needs->'hunger') = 'number'
        AND jsonb_typeof(needs->'energy') = 'number'
        AND jsonb_typeof(needs->'hygiene') = 'number'
        AND jsonb_typeof(needs->'mood') = 'number'
        AND needs->>'hunger' ~ '^(0|[1-9][0-9]{0,2})$'
        AND needs->>'energy' ~ '^(0|[1-9][0-9]{0,2})$'
        AND needs->>'hygiene' ~ '^(0|[1-9][0-9]{0,2})$'
        AND needs->>'mood' ~ '^(0|[1-9][0-9]{0,2})$'
        AND (needs->>'hunger')::INTEGER BETWEEN 0 AND 100
        AND (needs->>'energy')::INTEGER BETWEEN 0 AND 100
        AND (needs->>'hygiene')::INTEGER BETWEEN 0 AND 100
        AND (needs->>'mood')::INTEGER BETWEEN 0 AND 100
    ),
    ADD CONSTRAINT ck_pets_stats_contract CHECK (
        stats ?& ARRAY['str', 'agi', 'end', 'foc']::TEXT[]
        AND stats - ARRAY['str', 'agi', 'end', 'foc']::TEXT[] = '{}'::JSONB
        AND jsonb_typeof(stats->'str') = 'number'
        AND jsonb_typeof(stats->'agi') = 'number'
        AND jsonb_typeof(stats->'end') = 'number'
        AND jsonb_typeof(stats->'foc') = 'number'
        AND stats->>'str' ~ '^(0|[1-9][0-9]{0,9})$'
        AND stats->>'agi' ~ '^(0|[1-9][0-9]{0,9})$'
        AND stats->>'end' ~ '^(0|[1-9][0-9]{0,9})$'
        AND stats->>'foc' ~ '^(0|[1-9][0-9]{0,9})$'
        AND (stats->>'str')::NUMERIC BETWEEN 0 AND 4294967295
        AND (stats->>'agi')::NUMERIC BETWEEN 0 AND 4294967295
        AND (stats->>'end')::NUMERIC BETWEEN 0 AND 4294967295
        AND (stats->>'foc')::NUMERIC BETWEEN 0 AND 4294967295
    );

CREATE INDEX idx_pets_owner_active_created
    ON pets(owner_id, is_active DESC, created_at ASC, id ASC);
