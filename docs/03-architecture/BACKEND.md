# BACKEND: Сервер GOCHYA

> Сервер — авторитет для боя, экономики, генома, мутаций, IAP. Здесь — стек, сервисы, API-контракт, БД.

---

## 1. ТЕХНОЛОГИЧЕСКИЙ СТЕК

| Компонент | Технология |
|---|---|
| Язык | Go (рекомендуется, статическая типизация, отличный concurrency) |
| Web framework | chi или Echo |
| DB | PostgreSQL 16 + Redis 7 |
| Миграции | golang-migrate |
| ORM/Query | sqlc (генерация типизированного SQL) или pgx |
| Realtime | gorilla/websocket или nhooyr/websocket |
| IAP-валидация | App Store Server API, Google Play Developer API, Galaxy Store IAP |
| Auth | JWT + OAuth (Sign in with Apple, Samsung Account, Google) |
| Контейнер | Docker, оркестрация: Kubernetes или Fly.io/Render для старта |
| Observability | OpenTelemetry → Grafana/Tempo/Loki |
| CI/CD | GitHub Actions |

---

## 2. АРХИТЕКТУРА СЕРВИСОВ

```
                       ┌──────────────────────────┐
                       │  Load Balancer / API GW  │
                       └────────────┬─────────────┘
                                    │
              ┌─────────────────────┼─────────────────────┐
              │                     │                     │
        ┌─────┴─────┐        ┌──────┴──────┐       ┌──────┴─────┐
        │  REST API │        │  WebSocket  │       │  Webhooks  │ ← IAP callbacks
        │  (public) │        │  (realtime) │       │            │
        └─────┬─────┘        └──────┬──────┘       └──────┬─────┘
              │                     │                     │
              └─────────────────────┼─────────────────────┘
                                    │
        ┌───────────────────────────┼───────────────────────────┐
        │                           │                            │
   ┌────┴─────┐              ┌──────┴──────┐              ┌──────┴─────┐
   │  Auth    │              │  Profile    │              │ Inventory  │
   └──────────┘              └─────────────┘              └────────────┘
   ┌──────────┐              ┌─────────────┐              ┌────────────┐
   │  Combat  │              │Matchmaking  │              │  Seasons   │
   │ (core)   │              │             │              │            │
   └──────────┘              └─────────────┘              └────────────┘
   ┌──────────┐              ┌─────────────┐              ┌────────────┐
   │ Economy  │              │  Breeding   │              │  IAP       │
   │ (core)   │              │  (core)     │              │  Validator │
   └──────────┘              └─────────────┘              └────────────┘
   ┌──────────┐              ┌─────────────┐              ┌────────────┐
   │AntiCheat │              │ Leaderboards│              │ Analytics  │
   │          │              │ (Redis)     │              │  (events)  │
   └──────────┘              └─────────────┘              └────────────┘
```

Все сервисы — в одном монорепо (модульный монолит), общая БД. Микросервисы — если/когда нагрузка потребует.

---

## 3. ПОДКЛЮЧЕНИЕ SHARED CORE

- Сервер импортирует Shared Core (Go-cgo или через gRPC к WASM-сервису).
- **Рекомендация:** компилировать Rust-ядро в C-совместимую статическую библиотеку и подключать через cgo.
- Все вызовы формул идут в ядро — сервер не дублирует формулы.

```go
// #cgo LDFLAGS: /path/to/libgochya_core.a -lpthread -ldl -lm
// #include <gochya_core.h>
import "C"

func deriveTechnique(m PunchMetrics, h HeartRateEvidence) TechniqueStats {
    // versioned inputs include struct_size and schema_version
    // result is written into a caller-owned out parameter
    return call(C.gochya_derive_technique_v1, m, h)
}
```

Рабочий consumer находится в `server/internal/corebridge`: build tag
`gochya_core` включает статическую линковку, а сборка без него fail-closed
возвращает `ErrUnavailable` и не дублирует формулы.

---

## 4. ОСНОВНЫЕ СУЩНОСТИ БД (PostgreSQL)

Реализованный vertical slice не требует вручную подготовленных таблиц:
`server/migrations/000000_base.up.sql` создаёт `players`, `pets` и
`technique_cards`, а последующие миграции добавляют Dojo, auth session/login
nonces, device enrollment, loadout и профильные ограничения. Integration test
применяет всю цепочку к пустой schema, проверяет startup contract и затем
откатывает её в обратном порядке.

Миграция `000013_breeding` добавляет авторитетные яйца, item inventory/ledger
и breeding idempotency. Создание яйца блокирует игрока и обоих родителей,
проверяет adult Lv30, weakness, cooldown и родословную, затем одной транзакцией
фиксирует server seed, результат Rust Core, оба расхода и cooldown. Вылупление
блокирует egg row и сохраняет `hatched_pet_id`, поэтому повтор возвращает
существующего питомца.

Миграция `000014_onboarding` разрешает starter-яйцам не иметь родителей,
добавляет источник яйца и гарантирует не более одного starter-яйца на игрока.
Age gate хранит только производную категорию `under13 | 13plus`, но не дату
рождения. Выбор Fire/Water/Earth сериализуется блокировкой player row, вызывает
Rust Core по server seed и сохраняет яйцо, idempotency response и
пятисекундный tutorial timer одной транзакцией. До реализации проверяемого
parental-consent flow категория `under13` блокируется fail-closed.

Миграция `000015_care_sync` добавляет монотонную `care_revision`, авторитетный
cursor времени needs, четыре fixed-point остатка decay и sleeping window.
`care_operations` хранит hash и исходный результат на
`(player_id, operation_id)`. Reconcile блокирует player/pet/item rows,
пересчитывает decay/Weakness и care через ABI 2.3, затем одной транзакцией
изменяет pet snapshot, списывает предмет с балансирующей ledger-записью и
сохраняет идемпотентный ответ.

Миграция `000016_shop` добавляет неистекающий idempotency result покупки с
request hash и зафиксированной ценой. Магазин блокирует player и wallet,
атомарно списывает Koins, пополняет `player_items` и добавляет балансирующие
записи в оба ledger. Одинаковый retry не меняет wallet/inventory; конфликт
payload получает `idempotency_conflict`.

```sql
-- Профиль игрока
CREATE TABLE players (
    id              UUID PRIMARY KEY,
    username        TEXT UNIQUE NOT NULL,
    display_name    TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen       TIMESTAMPTZ,
    auth_method     TEXT NOT NULL,  -- apple | samsung | google
    auth_subject    TEXT NOT NULL,
    timezone        TEXT,
    -- Стрик активности: аргумент compute_vitality(), хранить было негде
    streak_days     INT NOT NULL DEFAULT 0,
    streak_last_day DATE,                -- для определения разрыва стрика
    UNIQUE(auth_method, auth_subject)
);

-- Refresh-токены (audit D5/B12: для refresh rotation — без этой таблицы политика инвалидации не работает)
CREATE TABLE refresh_tokens (
    id                  UUID PRIMARY KEY,
    family_id           UUID NOT NULL,
    player_id           UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    device_id           TEXT,                -- привязка к устройству (опц.)
    token_hash          BYTEA NOT NULL UNIQUE -- SHA-256; plaintext не хранится
                        CHECK (octet_length(token_hash) = 32),
    issued_at           TIMESTAMPTZ NOT NULL,
    expires_at          TIMESTAMPTZ NOT NULL,
    family_expires_at   TIMESTAMPTZ NOT NULL,
    revoked_at          TIMESTAMPTZ,
    replaced_by         UUID REFERENCES refresh_tokens(id),
    reuse_detected_at   TIMESTAMPTZ,
    CHECK (expires_at > issued_at),
    CHECK (family_expires_at >= expires_at)
);
CREATE INDEX idx_refresh_tokens_family ON refresh_tokens(family_id);
CREATE INDEX idx_refresh_tokens_player_expiry ON refresh_tokens(player_id, expires_at);

-- Питомцы
CREATE TABLE pets (
    id              UUID PRIMARY KEY,
    owner_id        UUID NOT NULL REFERENCES players(id),
    genome          JSONB NOT NULL,      -- сериализованный Genome из core
    name            TEXT,
    stage           TEXT NOT NULL,       -- egg | baby | teen | adult | premium
    level           INT NOT NULL DEFAULT 1,
    xp              BIGINT NOT NULL DEFAULT 0,
    needs           JSONB NOT NULL,      -- hunger, energy, hygiene, mood
    stats           JSONB NOT NULL,      -- STR, AGI, END, FOC
    generation      INT NOT NULL DEFAULT 0,
    is_active       BOOLEAN DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    parent_a_id     UUID REFERENCES pets(id),
    parent_b_id     UUID REFERENCES pets(id),
    -- Поля, которых требуют формулы ядра (CORE_FORMULAS §1.6, §3.1):
    last_bred_at        TIMESTAMPTZ,     -- кулдаун бридинга 24ч; NULL = не скрещивался
    needs_zero_since    TIMESTAMPTZ,     -- начало «нуля» потребности; Weakness через 6ч
    is_weak             BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE INDEX idx_pets_owner ON pets(owner_id);

-- Technique Cards
CREATE TABLE technique_cards (
    id              UUID PRIMARY KEY,
    owner_id        UUID NOT NULL REFERENCES players(id),
    card_data       JSONB NOT NULL,      -- TechniqueCard из core
    is_equipped     BOOLEAN DEFAULT FALSE,
    is_signature    BOOLEAN DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Инвентарь
CREATE TABLE inventory_items (
    id              UUID PRIMARY KEY,
    owner_id        UUID NOT NULL REFERENCES players(id),
    item_def_id     BIGINT NOT NULL,     -- каталожный номер (u32 в core, CORE_SPEC §3.7). Был TEXT — не совпадало с типом ядра
    quantity        INT NOT NULL DEFAULT 1,
    metadata        JSONB,
    acquired_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Экономика / валюты.
-- ВАЖНО: это ПРОЕКЦИЯ (кэш) поверх ledger'а, а не источник истины.
-- Прямой UPDATE запрещён — только через ledger.apply(), см. ANTICHEAT.md §6.
CREATE TABLE player_wallet (
    player_id       UUID PRIMARY KEY REFERENCES players(id),
    koins           BIGINT NOT NULL DEFAULT 0,
    vitality_daily  INT NOT NULL DEFAULT 0,
    vitality_date   DATE NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL
);

-- Double-entry ledger — ИСТОЧНИК ИСТИНЫ для всех валют.
-- Требование ANTICHEAT.md §6.1: «никогда не делаем wallet.koins += X напрямую».
-- Без этой таблицы проверка sum(transactions) == wallet невыполнима.
CREATE TABLE transactions (
    id              BIGSERIAL PRIMARY KEY,
    player_id       UUID NOT NULL REFERENCES players(id),
    currency        TEXT NOT NULL,        -- koins | vitality в MVP
    amount          BIGINT NOT NULL,      -- знаковая: + начисление, − списание
    counterparty    TEXT NOT NULL,        -- system:casual_rewards / shop / ...
    counterparty_amount BIGINT NOT NULL,  -- всегда -amount
    reason          TEXT NOT NULL,        -- duel_win | iap | gacha_pull | breed_cost | ...
    ref_id          TEXT,                 -- match_id / transaction_id стора / item_id
    idempotency_key TEXT NOT NULL,        -- защита от двойного начисления
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(player_id, idempotency_key)
);
CREATE INDEX idx_tx_player_currency ON transactions(player_id, currency);

-- Инвариант (cron-проверка из ANTICHEAT.md §6.2):
--   SELECT player_id, currency, SUM(amount) FROM transactions GROUP BY 1,2
--   должно совпадать с соответствующим полем player_wallet.

-- Матчи / бои
CREATE TABLE matches (
    id              UUID PRIMARY KEY,
    player_a        UUID NOT NULL REFERENCES players(id),
    player_b        UUID NOT NULL REFERENCES players(id),
    pet_a_id        UUID NOT NULL REFERENCES pets(id),
    pet_b_id        UUID NOT NULL REFERENCES pets(id),
    loadout_a       JSONB NOT NULL,
    loadout_b       JSONB NOT NULL,
    match_seed      BIGINT NOT NULL,
    result          JSONB NOT NULL,      -- MatchResult из core
    rating_delta_a  INT,
    rating_delta_b  INT,
    mode            TEXT NOT NULL,       -- casual | ranked | tournament
    season_id       INT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ELO / рейтинг
CREATE TABLE player_ratings (
    player_id       UUID PRIMARY KEY REFERENCES players(id),
    rating          INT NOT NULL DEFAULT 1000,
    matches_count   INT NOT NULL DEFAULT 0,
    season_id       INT NOT NULL,
    league          TEXT NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Сезоны
CREATE TABLE seasons (
    id              SERIAL PRIMARY KEY,
    name            TEXT NOT NULL,
    starts_at       TIMESTAMPTZ NOT NULL,
    ends_at         TIMESTAMPTZ NOT NULL,
    is_active       BOOLEAN DEFAULT FALSE
);

-- Дневная активность (для Vitality)
CREATE TABLE daily_activity (
    player_id       UUID NOT NULL REFERENCES players(id),
    date            DATE NOT NULL,
    snapshot        JSONB NOT NULL,      -- DailyActivitySnapshot (последний за день)
    vitality_awarded INT NOT NULL DEFAULT 0,   -- сколько УЖЕ начислено за этот день
    stat_gains      JSONB NOT NULL,
    source_metadata TEXT NOT NULL,       -- audit D5/T5: 'healthkit://watch' / 'samsung_health://watch' / 'google_fit://phone'
                                          -- ВНИМАНИЕ: это НЕ криптографическая подпись (см. ANTICHEAT.md §5.5),
                                          -- только metadata для дедупликации и приоритизации источников
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (player_id, date)
);

-- Яйца (эндпоинты GET /me/eggs, POST /me/eggs/:id/hatch существовали без таблицы)
CREATE TABLE eggs (
    id              UUID PRIMARY KEY,
    owner_id        UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    genome          JSONB NOT NULL,      -- BreedResult.genome из core
    parent_a_id     UUID NOT NULL REFERENCES pets(id),
    parent_b_id     UUID NOT NULL REFERENCES pets(id),
    incubate_until  TIMESTAMPTZ NOT NULL,
    breeding_seed   BYTEA NOT NULL,      -- ровно 8 byte, server-generated
    mutated_genes   INT NOT NULL,        -- 14-bit mask
    hatched_at      TIMESTAMPTZ,         -- NULL = ещё в инкубаторе
    hatched_pet_id  UUID UNIQUE REFERENCES pets(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_eggs_owner_incubating
    ON eggs(owner_id, created_at, id) WHERE hatched_at IS NULL;

-- Друзья (в MVP-скоупе, таблицы не было)
CREATE TABLE friendships (
    player_id       UUID NOT NULL REFERENCES players(id),
    friend_id       UUID NOT NULL REFERENCES players(id),
    status          TEXT NOT NULL,       -- pending | accepted | blocked
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (player_id, friend_id),
    CHECK (player_id <> friend_id)
);

-- Счётчики pity (pity_progress(&PlayerPulls) не имел хранилища)
CREATE TABLE gacha_pity (
    player_id       UUID NOT NULL REFERENCES players(id),
    banner_kind     TEXT NOT NULL,       -- standard | premium
    since_rare      INT NOT NULL DEFAULT 0,
    since_epic      INT NOT NULL DEFAULT 0,
    total_pulls     INT NOT NULL DEFAULT 0,
    PRIMARY KEY (player_id, banner_kind)
);

-- Battle Pass
CREATE TABLE battle_pass_progress (
    player_id       UUID NOT NULL REFERENCES players(id),
    season_id       INT NOT NULL REFERENCES seasons(id),
    level           INT NOT NULL DEFAULT 0,
    xp              INT NOT NULL DEFAULT 0,
    is_premium      BOOLEAN NOT NULL DEFAULT FALSE,
    claimed_levels  INT[] NOT NULL DEFAULT '{}',
    PRIMARY KEY (player_id, season_id)
);

-- Дневные / недельные задания
CREATE TABLE quests (
    id              UUID PRIMARY KEY,
    player_id       UUID NOT NULL REFERENCES players(id),
    quest_def_id    TEXT NOT NULL,
    period          TEXT NOT NULL,       -- daily | weekly | season
    progress        INT NOT NULL DEFAULT 0,
    target          INT NOT NULL,
    completed_at    TIMESTAMPTZ,
    claimed_at      TIMESTAMPTZ,
    expires_at      TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_quests_player_active ON quests(player_id) WHERE claimed_at IS NULL;

-- Турниры и сетка (эндпоинты /tournaments/* и тип Bracket существовали без таблиц)
CREATE TABLE tournaments (
    id              UUID PRIMARY KEY,
    season_id       INT NOT NULL REFERENCES seasons(id),
    name            TEXT NOT NULL,
    starts_at       TIMESTAMPTZ NOT NULL,
    ends_at         TIMESTAMPTZ NOT NULL,
    state           TEXT NOT NULL,       -- registration | running | finished
    max_players     INT NOT NULL
);

CREATE TABLE tournament_entries (
    tournament_id   UUID NOT NULL REFERENCES tournaments(id),
    player_id       UUID NOT NULL REFERENCES players(id),
    seed            INT,                 -- посев по рейтингу на момент старта
    eliminated_at   TIMESTAMPTZ,
    PRIMARY KEY (tournament_id, player_id)
);

-- Один ряд = один матч сетки (соответствует core-типу Bracket, CORE_SPEC §3.7)
CREATE TABLE tournament_brackets (
    tournament_id   UUID NOT NULL REFERENCES tournaments(id),
    round           SMALLINT NOT NULL,
    slot            SMALLINT NOT NULL,   -- позиция матча внутри раунда
    player_a        UUID REFERENCES players(id),   -- NULL = ещё не определён
    player_b        UUID REFERENCES players(id),
    winner_id       UUID REFERENCES players(id),   -- NULL пока не сыграно
    match_id        UUID REFERENCES matches(id),
    PRIMARY KEY (tournament_id, round, slot)
);

-- Логи античита (для расследований)
CREATE TABLE anticheat_events (
    id              BIGSERIAL PRIMARY KEY,
    player_id       UUID NOT NULL,
    event_type      TEXT NOT NULL,
    severity        TEXT NOT NULL,       -- info | suspect | rejected
    payload         JSONB NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### 4.1. Версионирование JSONB и миграции

- Каждый объект Shared Core хранится в envelope `{schema_version, data}`.
- Backend не десериализует JSONB без проверки версии.
- Core предоставляет последовательные чистые миграции `vN → vN+1`; пропуск версии выполняется цепочкой.
- DB rollout: dual-read/new-write → backfill → удаление старого reader отдельным релизом.
- Неизвестная новая версия возвращает `SCHEMA_MISMATCH`, а не partial object или default values.
- Snapshot хранит `core_build` и checksum канонического payload для диагностики.
- CI содержит fixture каждой поддерживаемой версии и проверяет upgrade до текущей версии byte-for-byte.

- Лидерборды — в Redis Sorted Sets (ZADD/ZREVRANGE).
- Сессии — Redis с TTL.
- Rate limits — Redis token bucket.

---

## 5. API КОНТРАКТ (REST, основные эндпоинты)

> ⚠️ **Аудит V5/D5:**
> - **Versioning:** все эндпоинты имеют префикс `/v1/` (ниже опущено для краткости).
> - **Idempotency:** `POST /me/pets/:id/feed`, `/breeding/breed`, `/me/techniques/equip`, `/dojo/submit` принимают заголовок `Idempotency-Key: <uuid>` — повторный запрос с тем же ключом возвращает сохранённый результат, не дублируя действие.
> - **Pagination:** все списки используют cursor-based: `?limit=20&cursor=<base64>` → `{ items, next_cursor }`.
> - **Error response body** (единая структура):
>   ```json
>   { "error": { "code": "evidence_invalid", "message": "recording duration is invalid", "details": {}, "request_id": "uuid" } }
>   ```
> - **Error codes:** стабильные `lower_snake_case`; HTTP status задаёт категорию,
>   а `code` — точную машинно-читаемую причину.

### Auth
```
POST   /v1/auth/apple/preflight  <empty body>             → { nonce, expiresAt }
POST   /v1/auth/apple         { identityToken, nonce,
                                deviceId? }                → { jwt, refreshToken, player }
POST   /v1/auth/samsung/preflight { redirectUri }          → { authorizationUrl, state,
                                                               nonce, codeVerifier, expiresAt }
POST   /v1/auth/samsung       { code, state, nonce,
                                codeVerifier, redirectUri,
                                deviceId? }                → { jwt, refreshToken, player }
POST   /v1/auth/google        { idToken }                 → { jwt, refreshToken, player }
POST   /v1/auth/refresh       { refreshToken }            → { jwt, refreshToken, accessTokenExpiresAt, refreshTokenExpiresAt }
POST   /v1/auth/logout        { refreshToken }            → 204   // revoke
```

Dojo HTTP-boundary уже содержит `JWTAuthenticator`: принимается только EdDSA,
ключ выбирается по подписанному `kid`, обязательны совпадающие issuer/audience,
`exp`, `iat`, `jti`, UUID в `sub` и `token_use=access`; lifetime по умолчанию
ограничен 15 минутами. `internal/auth` выпускает совместимые Ed25519 access
tokens и 256-bit opaque refresh tokens. Refresh живёт 30 дней, абсолютный
lifetime token family — 90 дней. Rotation выполняется транзакционно; повтор
отозванного токена фиксирует `reuse_detected_at` и отзывает всю family.
`/v1/auth/logout` также отзывает family и не раскрывает существование токена.
Непроверенный plaintext refresh-токена никогда не сохраняется.

Реализованы `/v1/auth/refresh` и `/v1/auth/logout`. Проверка provider credentials
для `/v1/auth/google` использует официальный Google ID-token validator и
дополнительно требует точный allowlist `aud`, Google `iss`, свежие `exp`/`iat`
и стабильный непустой `sub`. Email не является ключом аккаунта и не сохраняется.
Upsert по `(auth_method, auth_subject)` атомарен, после него выдаётся собственная
session pair.

Native Apple flow реализован отдельным challenge-response. Клиент сначала
вызывает `/v1/auth/apple/preflight`, устанавливает выданный 256-bit nonce
непосредственно в `ASAuthorizationAppleIDRequest.nonce`, затем отправляет Apple
identity token и тот же nonce в `/v1/auth/apple`. Challenge действует 5 минут и
атомарно потребляется один раз; в `auth_login_nonces` сохраняется только SHA-256.
Verifier принимает только RS256, выбирает ключ по `kid` из ротируемого Apple
JWK set, проверяет точные `iss=https://appleid.apple.com`, allowlist `aud`,
`exp`, `iat`, nonce и стабильный непустой `sub`. Неизвестный `kid` инициирует
контролируемое обновление короткого JWK-кэша. Email не является идентификатором
аккаунта и не сохраняется.

Этот endpoint предназначен для нативного `AuthenticationServices` flow. Для
будущего web Sign in with Apple потребуется дополнительно передавать и
проверять одноразовый authorization code через Apple token endpoint.
`HeaderAuthenticator` не является production-аутентификацией.

Samsung adapter следует OIDC discovery
`https://account.samsung.com/iam/.well-known/openid-configuration`.
`/v1/auth/samsung/preflight` принимает только точный зарегистрированный HTTPS
`redirectUri`, выдаёт authorization URL с `response_type=code`, `scope=openid`,
state, nonce и PKCE S256. State действует 5 минут и связан с nonce,
codeVerifier и redirect URI; PostgreSQL хранит только SHA-256 обоих bearer
значений. `/v1/auth/samsung` атомарно потребляет это состояние до внешнего
вызова, поэтому сбой после consume требует нового authorization flow.

Backend обменивает code на Samsung token endpoint через `client_secret_basic`;
secret никогда не передаётся клиенту. Возвращённый ID token принимается только
с RS256 и ключом из Samsung JWK set. Проверяются точные
`iss=https://account.samsung.com/iam`, client `aud`, `azp` при нескольких
audiences, `exp`, `iat`, nonce и стабильный `sub`. Provider access/refresh
tokens и email не сохраняются. Прежний контракт `{accessToken}` удалён: без
проверяемой привязки access token к GOCHYA client ID он допускал confused-deputy
login. Реальные partner credentials и PKCE проходят отдельный device gate перед
rollout; fallback, ослабляющий binding, запрещён.

### Device enrollment

```
POST   /v1/devices/preflight { deviceId, platform, appBuild }
    → { challenge, expiresAt }
POST   /v1/devices/register  { deviceId, platform, appBuild, challenge,
                               publicKey, attestation, proofSignature }
    → { deviceId, platform, registeredAt }
```

Оба endpoint требуют GOCHYA access token. Для Wear OS `platform` строго равен
`wear_os`, `appBuild` — allowlisted десятичный Play `versionCode`, `publicKey` —
32-byte Ed25519 key в unpadded base64url. Preflight создаёт 256-bit challenge с
TTL 5 минут и связывает его в PostgreSQL с player/device/platform/build; в БД
хранится только SHA-256 challenge.

Клиент подписывает новым device private key точные байты:

```text
canonical_json = {"version":1,"deviceId":...,"platform":...,
                  "appBuild":...,"challenge":...,"publicKey":...}
canonical = "gochya-device-enrollment-signature-v1" || 0x00 || canonical_json
proofSignature = base64url_no_padding(Ed25519.Sign(device_private_key, canonical))
requestHash = base64url_no_padding(SHA-256(
  "gochya-device-enrollment-play-integrity-v1" || 0x00 ||
  challenge || 0x00 || canonical
))
```

Play Integrity тем самым подтверждает разрешённый build/окружение для запроса,
который содержит тот же public key, а `proofSignature` подтверждает владение
соответствующим private key. После проверки attestation backend блокирует
challenge и строку игрока, затем одной транзакцией вставляет
`registered_devices` и помечает challenge использованным. Параллельный replay
не создаёт второй key binding. Уже активный идентичный key binding с новым
challenge возвращает исходный `registeredAt`; другой ключ, platform или
disabled state дают `409 device_key_conflict` и требуют будущего явного
recovery/revoke flow. Неявной ротации ключа нет.

Wear OS — единственная разрешённая платформа этого endpoint. watchOS остаётся
fail-closed с `platform_unsupported` до отдельного App Attest enrollment.
Остальные стабильные отказы: `enrollment_challenge_invalid`,
`enrollment_replay_detected`, `public_key_invalid`, `signature_invalid`,
`unsupported_build`, `attestation_invalid`, `attestation_unavailable`.

### Profile / Pets
```
GET    /me                                                → PlayerProfile
GET    /me/pets                                           → Pet[]
GET    /me/pets/:id                                       → Pet
POST   /me/pets/:id/activate                              → Pet (set active)
POST   /sync/commands               { deviceId, commands } → ReconcileResult
```

Реализованы `GET /v1/me`, `GET /v1/me/pets`,
`GET /v1/me/pets/:id`, `POST /v1/me/pets/:id/activate` и
`POST /v1/sync/commands`. Все endpoint'ы
требуют access token и никогда не читают питомца без фильтра по текущему
player ID. Список детерминированно возвращает активного питомца первым, затем
сортирует по `(created_at ASC, id ASC)`.

Активация естественно идемпотентна: строка игрока сериализует конкурентные
запросы, повтор уже активного питомца ничего не меняет. При реальной смене
предыдущий active flag снимается, новый устанавливается, а существующий
`player_loadouts.pet_id` и его `revision` обновляются в той же транзакции.
Миграция `000007_profile_pets_read` добавляет покрывающий индекс и DB constraints
для точного набора полей `needs` (`u8`, 0–100) и `stats` (`u32`). Decoder
повторно проверяет тот же контракт fail-closed. Стабильные отказы:
`profile_not_found`, `pet_not_found`, `pet_id_invalid`.

Offline care принимает `feed`, `clean`, `play`, `sleep` для одного питомца на
batch и проверяет `If-Match` против первой `baseRevision`. Команды старше 24
часов или более чем на пять минут из будущего отклоняются; client clock не
задаёт decay или длительность Sleep. Повтор operation ID с тем же payload не
повторяет эффект, а с другим payload возвращает `idempotency_conflict`.

### Training / Dojo
```
POST   /dojo/preflight       { deviceId, appBuild }      → { nonce, challenge, traceId, evidenceSchemaVersion, expiresAt }
POST   /dojo/submit          { deviceId, nonce, evidenceSchemaVersion, recordedAtMs, metrics,
                               heartEvidence, featureSummary, classifierVersion, appBuild,
                               attestation, payloadSignature }
                                                            → { card: TechniqueCard, evidenceVerdict, traceId }
GET    /me/techniques?limit=&cursor=                     → { items: TechniqueCard[], next_cursor? }
POST   /me/techniques/equip   { cardIds[], signatureIdx } → Loadout
GET    /me/loadout                                        → Loadout
```

Реализованный `GET /v1/me/techniques` требует access token и возвращает
`{ items, next_cursor? }`. `limit` по умолчанию 20, допустимый диапазон 1–100.
Cursor кодирует последнюю пару `(created_at, id)` как канонический unpadded
base64url; запрос сортируется по обоим полям в descending-порядке, поэтому
карты с одинаковым timestamp не пропускаются и не дублируются между страницами.
PostgreSQL использует индекс `(owner_id, created_at DESC, id DESC)` из migration
`000005_technique_pagination`. Поля `id`, `ownerId`, `createdAt` берутся из
авторитетных колонок, а не из JSONB; время нормализуется в UTC независимо от
timezone соединения. Attestation, подпись и privacy-safe feature summary в
inventory response не попадают.

Реализованный `POST /v1/me/techniques/equip` требует access token,
`Idempotency-Key: <uuid>`, ровно пять уникальных UUID принадлежащих игроку карт
и `signatureIdx` в диапазоне 0–4. Активный питомец определяется сервером.
Атомарная транзакция блокирует игрока, проверяет питомца и владение всеми
картами, записывает авторитетный `player_loadouts` и обновляет карточные флаги
как read projection. Ответ и `GET /v1/me/loadout` имеют вид
`{ petId, cardIds, signatureIdx, revision, updatedAt }`; `revision` монотонно
растёт. Идентичный retry в течение 24 часов возвращает исходный ответ без новой
ревизии, а повтор ключа с другим запросом возвращает
`409 idempotency_conflict`. Остальные доменные отказы:
`active_pet_required`, `loadout_cards_invalid`, `loadout_not_found`.
Схему и отдельное пространство idempotency создаёт
`000006_loadouts`, поэтому equip-ключ не может пересечься с типом ответа Dojo.

> ⚠️ **Контракт приведён в соответствие с `ANTICHEAT.md` §3 (исправление аудита).**
> - `preflight` **выдаёт серверный `nonce`** (TTL 5 мин, одноразовый) — без него
>   replay-detection из §3.3 нереализуема. Прежде эндпоинт возвращал
>   `{ baseline, contactConfidence }`, хотя обе величины измеряются **на устройстве**
>   и серверу в этот момент неизвестны.
> - `submit` принимает `nonce`, versioned `featureSummary`, build/classifier version,
>   platform attestation и подпись канонического payload (§3.3a–b). Прежнее поле
>   `signalHash` убрано: сервер сам считает
>   `replay_hash = SHA-256(nonce ‖ schemaVersion ‖ metrics ‖ heartEvidence ‖ featureSummary)`, доверять хэшу
>   от клиента бессмысленно.
> - `signatureId` → `signatureIdx` (0..4): signature — свойство лоадаута,
>   а не карты (`CORE_SPEC.md` §3.6).
>
> Возможные коды отказа `submit`: `heart_rejected`, `attestation_invalid`,
> `attestation_unavailable`,
> `signature_invalid`, `evidence_invalid`, `replay_detected`, `nonce_invalid`,
> `rate_limited`, `daily_limit`, `idempotency_conflict`, `core_unavailable`.
>
> Реализованный vertical slice находится в `server/internal/dojo`. Он строго
> отклоняет неизвестные JSON-поля и тела больше 64 KiB, использует Ed25519 и
> сохраняет idempotency result до проверки использованного nonce, поэтому
> безопасный сетевой retry возвращает исходную карту. `MemoryStore` служит
> эталоном для unit-тестов, а `PostgresStore` реализует production-семантику:
> row lock игрока сериализует дневной/минутный лимит, nonce блокируется
> `FOR UPDATE`, а карта, audit, replay hash, idempotency result и `used_at`
> фиксируются одной транзакцией. Bearer nonce в БД не хранится — только SHA-256.
> Layout находится в `server/migrations/000001_dojo.up.sql`.
> Preflight создаёт отдельный UUID `traceId`, не являющийся bearer-секретом.
> Сервер восстанавливает его по хэшу nonce и передаёт через Go context в
> attestation verifier, Rust Core и PostgreSQL Store. Тот же ID сохраняется в
> `dojo_submission_audit`, idempotent response и возвращается клиенту в
> `traceId`/`X-Trace-ID`; `X-Request-ID` уникален для каждой HTTP-попытки.
> Ошибка после успешного распознавания nonce также содержит `X-Trace-ID`.
>
> Wear OS использует Play Integrity Standard API. Клиент передаёт
> `attestation.provider = "play_integrity_standard"`, а `appBuild` содержит
> десятичный Play `versionCode`. Content binding:
>
> ```text
> canonical = canonical JSON всех подписываемых полей submit
> requestHash = base64url_no_padding(SHA-256(
>   "gochya-dojo-play-integrity-v1" || 0x00 ||
>   challenge || 0x00 || canonical
> ))
> ```
>
> Сервер отправляет encrypted token в
> `playintegrity.googleapis.com/v1/{package}:decodeIntegrityToken` через
> Application Default Credentials и scope `playintegrity`. Policy требует
> совпадающие request package/hash/version/certificate, свежий timestamp,
> `PLAY_RECOGNIZED`, `LICENSED` и минимум `MEETS_DEVICE_INTEGRITY`.
> Testing verdict запрещён без явного staging-флага. Сетевой/Google/ADC сбой
> даёт `503 attestation_unavailable` и допускает retry; отрицательный verdict —
> `401 attestation_invalid`.
> Чтобы параллельные submit/retry не декодировали один Standard token несколько
> раз, процесс объединяет одинаковые запросы и на две минуты кэширует
> проверенный результат. Ключ — SHA-256 от encrypted token и `requestHash`;
> raw token не сохраняется, временные ошибки в кэш не попадают.
>
> HTTP-слой принимает интерфейс аутентификации. `HeaderAuthenticator` с
> `X-Player-ID` допустим только в тестах/закрытом staging; production обязан
> передать `JWTAuthenticator`. Production передаёт настроенный
> `PlayIntegrityVerifier`; `RejectingAttestationVerifier` остаётся безопасным
> fallback при отсутствии конфигурации.

### PvP
```
POST   /matchmaking/queue    { mode: "casual" }          → { matchId, status }
GET    /match/:id                                        → MatchResult
POST   /match/:id/confirm                                 → { rewards, card? }
GET    /me/matches/history?limit=20                      → MatchSummary[]
```

Реализованный `POST /v1/matchmaking/queue` не принимает клиентский loadout:
сервер читает обе авторитетные строки `player_loadouts`, pet stats/genome/mood
и пять принадлежащих владельцу card snapshots. Запрос требует access token,
`Idempotency-Key: <uuid>` и пока поддерживает только `casual`. В одной
PostgreSQL-транзакции фиксируются обе loadout revision, случайный 63-bit seed,
snapshot входов и результат native `gochya_simulate_combat_v1`. Повтор ключа
возвращает исходный `matchId` без второго Core-вызова.

`GET /v1/match/:id` возвращает сохранённый replay только одному из двух
участников. Миграция `000008_casual_matches` создаёт `matches` и отдельную
idempotency table. Текущий MVP-matcher синхронно выбирает первый доступный
не-weak loadout и сериализует создание матчей PostgreSQL advisory lock.
`GET /v1/me/matches/history` также ограничен участником, принимает `limit`
`1..100` (по умолчанию 20), сортирует по `created_at DESC, id DESC` и возвращает
`[{ id, opponentId, mode, outcome, createdAt }]`, где `outcome` вычисляется
сервером относительно JWT subject (`win | loss | draw`).

`POST /v1/match/:id/confirm` принимает пустое тело и не принимает outcome,
сумму или валюту от клиента. Сервер читает сохранённый `MatchResult` и
возвращает
`{ matchId, outcome, rewards: [{ currency, amount }], card?, confirmedAt }`.
Нормативные суммы — 30/20/10 Koins за win/draw/loss
(`CORE_FORMULAS.md` §6.3a). Награждаются первые 10 матчей игрока за UTC-день в
порядке `(created_at ASC, id ASC)`; порядок confirm eligibility не меняет.
Первая casual-победа игрока в том же стабильном порядке дополнительно выдаёт
одну Technique Card с потолком Epic. UUID и seed назначает сервер, stats
вычисляет Rust Core; элемент берётся из сохранённого winning loadout snapshot.
Каждый участник подтверждает матч отдельно. Повтор и конкурентный retry
возвращают исходные `confirmedAt` и `card`, не создавая вторую карту или
транзакцию.

Миграция `000009_match_rewards` создаёт `player_wallet`, двухсторонние
`transactions` (`amount + counterparty_amount = 0`) и
`match_confirmations`. Confirmation, ledger и wallet projection фиксируются
одной PostgreSQL-транзакцией. Миграция `000012_pvp_card_rewards` сохраняет
card ID и 8-byte seed рядом с confirmation; выдача карты входит в ту же
транзакцию. Redis queue и ожидание до 60 секунд остаются
следующим rollout; endpoint не имитирует их готовность. Стабильные отказы:
`loadout_required`, `pet_weak`, `no_opponent`, `match_not_found`,
`idempotency_conflict`, `core_unavailable`.

### Breeding
```
POST   /breeding/breed       { parentA, parentB, catalysts[] }
                                                            → { eggId, incubateUntil }
GET    /me/eggs                                          → Egg[]
POST   /me/eggs/:id/hatch                                → Pet
GET    /me/pets/:id/lineage                              → LineageTree
```

### Onboarding
```
POST   /me/onboarding/age-gate
       { birthDate }                  → { status, coppaRestricted, recordedAt }
POST   /me/onboarding/starter-egg
       { element: fire|water|earth }  → { eggId, element, incubateUntil }
```

Оба POST требуют UUID `Idempotency-Key`. Дата рождения используется только
внутри первого запроса для вычисления категории и не записывается в БД.

### Shop / Gacha / IAP
```
GET    /shop                                             → Catalog
POST   /shop/buy              { itemId, quantity }       → Purchase
GET    /me/items                                        → { koins, items[] }
POST   /gacha/pull            { bannerId, count }        → PullResult[]
GET    /gacha/banners                                    → Banner[]

POST   /iap/validate-apple    { transactionId }          → { ok, rewards }
POST   /iap/validate-google    { purchaseToken }          → { ok, rewards }
POST   /iap/validate-galaxy    { purchaseId }             → { ok, rewards }
POST   /webhooks/apple        ← App Store Server Notifications V2
POST   /webhooks/google       ← RTDN
POST   /webhooks/galaxy       ← IAP webhook
```

Реализованы `GET /v1/shop`, `POST /v1/shop/buy` и `GET /v1/me/items`.
Bootstrap-каталог содержит только численно согласованные Koins SKU:
`apple=20`, `steak=80`, `energy_drink=50`, `soap=30`, `shampoo=60`,
`love_crystal=200`. Purchase принимает количество 1–100 и UUID
`Idempotency-Key`; currency и цену клиент не передаёт. Один player lock
сериализует магазин с care/breeding, а wallet projection, currency ledger,
item projection, item ledger и сохранённый response изменяются в одной
PostgreSQL-транзакции. `GET /v1/me/items` возвращает только положительные
остатки в стабильном порядке `item_id ASC`.

Этот bootstrap-каталог разблокирует текущие care/breeding flows, но не считается
готовностью косметической части MVP: три cosmetic asset ID и их Koins-цены
должны быть сначала зафиксированы в `CONTENT_MANIFEST.md`.

### Seasons / Leaderboards
```
GET    /seasons/current                                  → Season
GET    /leaderboard/global?season=&league=               → Entry[]
GET    /leaderboard/friends                              → Entry[]
GET    /me/season-progress                               → SeasonProgress
```

### Activity Sync
```
POST   /sync/activity         { snapshot, sourceMetadata } → { vitality, statGains }
GET    /me/activity/week                                   → DailyActivity[]
POST   /me/activity/reward                                 → { date, card, awarded }
```

Реализованный `POST /v1/sync/activity` принимает snapshot schema `1`:
`timestampMillis`, steps/sleep/calories, quality/stress, агрегаты HR, floors,
stand hours и до восьми workouts `{kind,durationMinutes,calories}`. Поле
`sourceMetadata` allowlisted и нормализуется, но не считается подписью или
attestation. Ответ дополнительно возвращает `date`, adaptive `goals`,
`vitalityDelta`, `statGainDeltas` и `snapshotAccepted`, чтобы клиент отличал
дневной total от реально применённой этой попыткой дельты.

Текущий production slice принимает только текущий день в сохранённой timezone
игрока. PostgreSQL player lock сериализует конкурентные sync; SHA-256 fingerprint
повторного snapshot делает его no-op до вызова Core. Первый sync фиксирует
активного питомца на весь день. Adaptive goals рассчитываются Core по среднему
предыдущих 14 дней. Vitality применяет только положительную дельту и пишет
двухстороннюю ledger-запись; коррекция вниз её не отзывает. Stat gains
reconcile'ятся до нового дневного total, причём фактически применённая отрицательная
дельта ограничивается нулём pet stat и сохраняется отдельно для корректного
следующего пересчёта.

`GET /v1/me/activity/week` возвращает только строки текущего JWT player за
текущий local day и шесть предыдущих calendar days, отсортированные по дате
возрастания. В projection входят canonical snapshot, computed/awarded vitality,
stat gains, goals, source metadata и UTC `updatedAt`; fingerprint и внутренний
`stat_gains_applied` клиенту не раскрываются. Дни без sync не заполняются
синтетическими нулевыми объектами.

`POST /v1/me/activity/reward` с пустым body выдаёт карту после достижения
100 Vitality за текущий локальный день. Потолок редкости — Rare. UUID и seed
создаёт сервер, stats детерминированно вычисляет Rust Core, а карта, seed и
связь с `daily_activity` сохраняются в одной транзакции. Первичный ответ имеет
`awarded: true`; любой последовательный или конкурентный повтор возвращает ту
же карту с `awarded: false`, не создавая вторую строку inventory. Исправление
activity snapshot ниже 100 после уже состоявшейся выдачи карту не отзывает.

> ⚠️ Поле называлось `deviceSig`, что подразумевало криптографическую подпись.
> Её не существует: HealthKit и Samsung Health подписанных агрегатов сторонним
> приложениям не выдают (`ANTICHEAT.md` §5.5). Передаётся строка-источник
> (`healthkit://watch`, `samsung_health://watch`, `google_fit://phone`),
> и используется она только для дедупликации и приоритизации источников,
> **не как доказательство подлинности**.

> **Идемпотентность — обязательна.** Клиент синхронизируется раз в час, и снапшот за день
> приходит многократно с растущими значениями. Начислять `compute_vitality(snapshot)`
> при каждом вызове нельзя: игрок получит валюту заново на каждом sync'е.
>
> Протокол начисления (в одной транзакции):
> ```
> total := compute_vitality(snapshot, goals, streak_days)   -- пересчёт за ВЕСЬ день
> delta := max(0, total - daily_activity.vitality_awarded)  -- начисляем только прирост
> if delta > 0:
>     ledger.apply(player, 'vitality', +delta,
>                  idempotency_key = 'vitality:' || player_id || ':' || date || ':' || total)
>     daily_activity.vitality_awarded := total
> ```
> `delta` не может быть отрицательной: при коррекции данных здоровья вниз ранее
> начисленное **не отзывается** (иначе у игрока пропадает уже потраченная валюта).
> Дневной кэп обеспечивает `compute_vitality`, а не эндпоинт.

### Tournaments
```
GET    /tournaments/active                               → Tournament[]
POST   /tournaments/:id/join                            → Bracket
GET    /tournaments/:id/bracket                         → Bracket
```

---

## 6. WEBSOCKET (Realtime)

```
WS  /ws/matchmaking         → события: opponent_found, match_ready
WS  /ws/tournament/:id      → события: round_start, round_end, bracket_update
WS  /ws/spectate/:matchId   → live трансляция боя (для топовых матчей)
```

---

## 7. МАТЧМЕЙКИНГ

```go
// псевдокод
const (
    baseTolerance = 50
    tolerancePerSec = 2      // допуск расширяется со временем ожидания
    maxTolerance  = 600
    queueDeadline = 60 * time.Second
)

func findMatch(ctx context.Context, p Player, l Loadout, mode Mode) (Match, error) {
    power := core.EffectivePower(l)
    enqueue(mode, p.ID, power)
    defer dequeue(mode, p.ID)

    deadline := time.Now().Add(queueDeadline)
    for {
        // допуск ПЕРЕСЧИТЫВАЕТСЯ на каждой итерации, иначе он никогда не растёт
        waited := time.Since(deadline.Add(-queueDeadline)).Seconds()
        tol := min(baseTolerance+int(waited)*tolerancePerSec, maxTolerance)

        if opp := redisSearchOpponent(mode, power, tol, p.ID); opp != nil {
            return createMatch(p, opp, mode)
        }
        if time.Now().After(deadline) {
            return Match{}, ErrNoOpponent   // клиент предложит бой с ботом
        }
        select {
        case <-ctx.Done():                  // игрок ушёл из очереди
            return Match{}, ctx.Err()
        case <-time.After(2 * time.Second):
        }
    }
}
```

> ⚠️ Прежний псевдокод считал `tolerance` **один раз до цикла**, поэтому допуск
> не расширялся никогда, а по таймауту вызывал `expandTolerance()`, возвращая его
> результат как `Match` — несовместимые типы. Кроме того, не было ни выхода по отмене,
> ни исключения самого игрока из результатов поиска.

- Redis ZSET `matchmaking_queue_<mode>` с ключом `effectivePower`.
- Поиск в радиусе `[power-tolerance, power+tolerance]`.
- **Кроссплатформенный единый пул:** watchOS ↔ Wear OS ↔ **телефон** в одном ZSET.

> **Единый пул без платформенного коэффициента (проектное решение).** Ключ очереди —
> `effectivePower`, и **никакого множителя «телефон/часы» к нему не применяется.**
> Причины:
> - `effectivePower` уже включает силу карт (`RARITY_STAT_MULT`), поэтому игрок
>   со слабыми картами естественно получает более низкий power и соперника такой же
>   силы — доступ к записи удара учитывается автоматически, без отдельной ручки.
> - Платформенный коэффициент был бы эксплуатируем: игрок подавал бы лоадаут
>   с той платформы, которой достался бонус.
> - Не дробить пул критично для бюджета `matchmaking ≤ 30 сек median` — при базе
>   нишевой категории два раздельных пула удвоили бы время подбора.
>
> Асимметрия сенсоров между самими часами (watchOS vs Wear OS) — отдельный вопрос
> паритета `qualityScore`, решается нормализацией в ядре, а не в матчмейкинге
> (`MECHANIC_ML_CLASSIFIER.md` §4). Телефон в этой асимметрии не участвует — он
> карты записью не создаёт.

---

## 8. IAP-ВАЛИДАЦИЯ

### Apple (StoreKit 2)
- При получении `Transaction` от StoreKit 2 — отправить `transactionId` на сервер.
- Сервер через **App Store Server API** (`/transactions/{transactionId}`) проверяет валидность.
- Подписка на **App Store Server Notifications V2** — узнавать о рефандах, истечениях.

### Google Play Billing
- При покупке клиент получает `purchaseToken`.
- Сервер через **Google Play Developer API** (`purchases.subscriptions.get` / `purchases.products.get`) валидирует.
- Подписка на **Real-time developer notifications (RTDN)** от Pub/Sub.

### Galaxy Store IAP
- При покупке клиент получает `purchaseId`.
- Сервер через **Galaxy Store IAP Server API** валидирует (`verifyPurchase`).
- ⚠️ Покрытие server API ограничено по сравнению с Apple/Google — тестировать edge cases (refunds, cancellations) вручную.
- ⚠️ **Galaxy Store IAP на часах не сертифицирован Samsung** — покупки идут через companion/телефон.

### Webhook-верификация (audit D5 — критично для безопасности)
- **App Store Server Notifications V2:** каждый webhook — это **JWS** (signed JWT). Сервер ОБЯЗАН:
  1. Верифицировать signature по Apple's certificate chain (загружается с `Apple Root CA`).
  2. Проверить `x5c` header chain.
  3. Декодировать payload, проверить `notificationType` и `signedTransactionInfo` (отдельный JWS внутри).
- **Google RTDN:** приходит через Pub/Sub. Верификация:
  1. Pub/Sub message OIDC token проверяется через Google's public keys.
  2. `messageId` идемпотентен (хранить в Redis 90 дней).
- **Galaxy Store webhook:** проверка HMAC-SHA256 с shared secret (задаётся в Galaxy Store developer console).
- **Retry-политика:** все три стора ретраят webhook при не-2xx. Сервер должен быть идемпотентен по `transactionId`/`messageId`.

### Идемпотентность
- Каждая покупка хранится с уникальным `transactionId`, повторная валидация — no-op.
- Награды выдаются ровно один раз.
- Webhooks: повторная доставка того же `transactionId` — no-op (проверка по БД перед начислением).

---

## 9. АНТИЧИТ НА СЕРВЕРЕ

(подробно — `docs/05-security/ANTICHEAT.md`)

- Бой: только сервер через core, клиент лишь отображает.
- Запись удара: серверный вердикт на основе метрик + heart gate + signal hash.
- Активность: дедупликация, типы активности, soft cap.
- Экономика: double-entry ledger, аудит каждой транзакции.
- Rate limiting на все эндпоинты (Redis token bucket).
- Device fingerprint для выявления ботов/эмуляторов.

---

## 10. СЕЗОНЫ И РЕЙТИНГ

- Сезон 4 недели. В конце:
  - Награды по финальной лиге (Crowns + эксклюзивная косметика).
  - Squish: рейтинг × 0.75, лиги пересчитаны.
  - Reset BP.
- Лиги: Bronze (0–1199), Silver (1200–1499), Gold (1500–1799), Platinum (1800–2099), Diamond (2100–2399), Master (2400+).
- ELO-формула:
```
expectedA = 1 / (1 + 10^((ratingB - ratingA) / 400))
deltaA    = K · (scoreA - expectedA)        // K=32 для новичков, 24 после 30 игр
```

---

## 11. OBSERVABILITY

- **Metrics:** Prometheus → Grafana (DAU, latency, error rate, IAP-конверсия).
- **Tracing:** OpenTelemetry → Tempo (каждая дуэль — span).
- **Logs:** структурный логгинг → Loki.
- **Alerts:** SLO-based (p95 latency > 500ms, error rate > 1%, matchmaking wait > 30s).

---

## 12. PERFORMANCE BUDGET (Backend)

| Метрика | Цель |
|---|---|
| API p95 latency | ≤ 300 мс (кроме боя/матчмейкинга) |
| Matchmaking wait | ≤ 30 сек median |
| Uptime | 99.9% |
| DB connections pool | ≤ 80% использования |
| WebSocket connections per node | ≤ 10 000 |

---

## 13. СВЯЗАННЫЕ ДОКУМЕНТЫ

- `ARCHITECTURE.md` — общая картина.
- `docs/04-core/CORE_SPEC.md` — формулы, вызываемые сервером.
- `docs/05-security/ANTICHEAT.md` — античит.
- `docs/05-security/BALANCE.md` — числа баланса.
