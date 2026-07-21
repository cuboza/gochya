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
// #cgo LDFLAGS: -lgochya_core
// #include <gochya_core.h>
import "C"

func qualityScore(m PunchMetrics, h HeartRateEvidence) float32 {
    cm := m.toC()
    ch := h.toC()
    return float32(C.gochya_quality_score(&cm, &ch))
}
```

---

## 4. ОСНОВНЫЕ СУЩНОСТИ БД (PostgreSQL)

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
    item_def_id     TEXT NOT NULL,       -- ссылка на каталог
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
    gems            BIGINT NOT NULL DEFAULT 0,
    vitality_daily  INT NOT NULL DEFAULT 0,
    crowns          INT NOT NULL DEFAULT 0,
    vitality_date   DATE NOT NULL
);

-- Double-entry ledger — ИСТОЧНИК ИСТИНЫ для всех валют.
-- Требование ANTICHEAT.md §6.1: «никогда не делаем wallet.koins += X напрямую».
-- Без этой таблицы проверка sum(transactions) == wallet невыполнима.
CREATE TABLE transactions (
    id              BIGSERIAL PRIMARY KEY,
    player_id       UUID NOT NULL REFERENCES players(id),
    currency        TEXT NOT NULL,        -- koins | gems | vitality | crowns
    amount          BIGINT NOT NULL,      -- знаковая: + начисление, − списание
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
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (player_id, date)
);

-- Яйца (эндпоинты GET /me/eggs, POST /me/eggs/:id/hatch существовали без таблицы)
CREATE TABLE eggs (
    id              UUID PRIMARY KEY,
    owner_id        UUID NOT NULL REFERENCES players(id),
    genome          JSONB NOT NULL,      -- EggGenome из core
    parent_a_id     UUID REFERENCES pets(id),
    parent_b_id     UUID REFERENCES pets(id),
    incubate_until  TIMESTAMPTZ NOT NULL,
    hatched_at      TIMESTAMPTZ,         -- NULL = ещё в инкубаторе
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_eggs_owner ON eggs(owner_id) WHERE hatched_at IS NULL;

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

- Лидерборды — в Redis Sorted Sets (ZADD/ZREVRANGE).
- Сессии — Redis с TTL.
- Rate limits — Redis token bucket.

---

## 5. API КОНТРАКТ (REST, основные эндпоинты)

### Auth
```
POST   /auth/apple         { identityToken }           → { jwt, player }
POST   /auth/samsung       { accessToken }             → { jwt, player }
POST   /auth/google        { idToken }                 → { jwt, player }
POST   /auth/refresh       { refreshToken }            → { jwt }
```

### Profile / Pets
```
GET    /me                                                → PlayerProfile
GET    /me/pets                                           → Pet[]
GET    /me/pets/:id                                       → Pet
POST   /me/pets/:id/activate                              → Pet (set active)
POST   /me/pets/:id/feed            { itemId }            → Pet (updated needs)
POST   /me/pets/:id/clean                                 → Pet
POST   /me/pets/:id/play                                  → Pet
POST   /me/pets/:id/sleep                                 → Pet
```

### Training / Dojo
```
POST   /dojo/preflight       { }                         → { baseline, contactConfidence }
POST   /dojo/submit          { metrics, heartEvidence, signalHash }
                                                            → TechniqueCard
GET    /me/techniques                                    → TechniqueCard[]
POST   /me/techniques/equip   { cardIds[], signatureId } → Loadout
```

### PvP
```
POST   /matchmaking/queue    { loadout, mode }           → { matchId }
GET    /match/:id                                        → MatchResult
POST   /match/:id/confirm                                 → { rewards }
GET    /me/matches/history?limit=20                      → MatchSummary[]
```

### Breeding
```
POST   /breeding/breed       { parentA, parentB, catalysts[] }
                                                            → { eggId, incubateUntil }
GET    /me/eggs                                          → Egg[]
POST   /me/eggs/:id/hatch                                → Pet
GET    /me/pets/:id/lineage                              → LineageTree
```

### Shop / Gacha / IAP
```
GET    /shop                                             → Catalog
POST   /shop/buy              { itemId, currency, qty }  → Transaction
POST   /gacha/pull            { bannerId, count }        → PullResult[]
GET    /gacha/banners                                    → Banner[]

POST   /iap/validate-apple    { transactionId }          → { ok, rewards }
POST   /iap/validate-google    { purchaseToken }          → { ok, rewards }
POST   /iap/validate-galaxy    { purchaseId }             → { ok, rewards }
POST   /webhooks/apple        ← App Store Server Notifications V2
POST   /webhooks/google       ← RTDN
POST   /webhooks/galaxy       ← IAP webhook
```

### Seasons / Leaderboards
```
GET    /seasons/current                                  → Season
GET    /leaderboard/global?season=&league=               → Entry[]
GET    /leaderboard/friends                              → Entry[]
GET    /me/season-progress                               → SeasonProgress
```

### Activity Sync
```
POST   /sync/activity         { snapshot, deviceSig }    → { vitality, statGains }
GET    /me/activity/week                                → DailyActivity[]
```

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
func findMatch(player Player, loadout Loadout, mode Mode) (Match, error) {
    power := core.EffectivePower(loadout)
    tolerance := 50 + (queueTimeSeconds(player) * 2)  // расширяется со временем

    for {
        opponent := redisSearchOpponent(power, tolerance)
        if opponent != nil {
            return createMatch(player, opponent, mode)
        }
        time.Sleep(2 * time.Second)
        if queueTimeout(player) { return expandTolerance() }
    }
}
```

- Redis ZSET `matchmaking_queue_<mode>` с ключом `effectivePower`.
- Поиск в радиусе `[power-tolerance, power+tolerance]`.
- Кроссплатформенный: watchOS ↔ Wear OS ↔ companion в одном пуле.

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

### Идемпотентность
- Каждая покупка хранится с уникальным `transactionId`, повторная валидация — no-op.
- Награды выдаются ровно один раз.

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
