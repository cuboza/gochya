-- CORE_FORMULAS.md §1.8: одна ночь сна владельца, ещё не применённая к питомцу.
--
-- Активность кладёт ночь сюда, уход применяет её в своей транзакции — там, где
-- уже удерживается блокировка питомца, ревизия и remainder-ы распада. Так ни
-- один домен не начинает знать о другом: оба и так пишут в `pets`.
ALTER TABLE pets
    ADD COLUMN pending_rest_minutes INTEGER NOT NULL DEFAULT 0
        CHECK (pending_rest_minutes BETWEEN 0 AND 65535),
    ADD COLUMN pending_rest_quality SMALLINT NOT NULL DEFAULT 0
        CHECK (pending_rest_quality BETWEEN 0 AND 100);

-- Ночь применяется один раз: повторный синк той же даты не начисляет Energy
-- второй раз, даже если снимок уточнился.
ALTER TABLE daily_activity
    ADD COLUMN rest_applied BOOLEAN NOT NULL DEFAULT FALSE;
