# GOCHYA — Архитектура

> Общая архитектура проекта «общее ядро + нативные оболочки». Специфика платформ — в `CLIENT_*.md`, сервер — в `BACKEND.md`.

---

## 1. КЛЮЧЕВОЙ ПРИНЦИП

```
        ┌─────────────────────────────────────────┐
        │  SHARED GAME CORE (Rust)                │  ← вся игровая логика
        │  • Genome, Pet, TechniqueCard, Combat   │     детерминированная
        │  • Formulas (баланс, урон, мутации)     │     тестируется без UI
        │  • Античит-хелперы                      │     компилируется под 5 таргетов
        └─────────────────────────────────────────┘
              ▲           ▲           ▲           ▲
              │           │           │           │
   ┌──────────┴──┐ ┌──────┴────┐ ┌────┴─────┐ ┌───┴────┐
   │  watchOS    │ │  Wear OS  │ │Comp-     │ │ Server │
   │  (SwiftUI + │ │  (Kotlin + │ │anion     │ │ (Go +  │
   │  SpriteKit) │ │  Filament)│ │(Flutter) │ │  cgo)  │
   └─────────────┘ └───────────┘ └──────────┘ └────────┘
```

- **Общее ядро** — source of truth для всей логики. Клиенты и сервер импортируют его.
- **Клиенты тонкие** — только UI, ввод, рендер, чтение сенсоров, отправка намерений на сервер.
- **Сервер — авторитет** для боя, экономики, генома, мутаций, IAP.

> **Телефон — полноценный клиент, не companion.** Флаттер-клиент исполняет то же ядро
> через `dart:ffi` и проходит игру целиком без часов (`CLIENT_COMPANION.md`). Часы —
> опциональный премиум-клиент; их эксклюзив — запись ударов (Dojo). Все три клиента
> равноправны перед сервером-авторитетом и делят единый PvP-пул.

---

## 2. КОМПОНЕНТЫ СИСТЕМЫ

| Компонент | Технология | Роль |
|---|---|---|
| **Shared Core** | Rust | Игровая логика, формулы, типы |
| **watchOS client** | Swift, SwiftUI, WatchKit, **SpriteKit** (soft 3D), CoreMotion, HealthKit, StoreKit 2 | Apple Watch приложение |
| **Wear OS client** | **Kotlin, Jetpack Compose for Wear OS, Filament** (PBR), SensorManager, Health Services | Galaxy Watch приложение |
| **Companion (iOS/Android)** | Flutter | Магазин, турниры, инвентарь, родословная, рынок |
| **Backend** | Go + **cgo** (нативная линковка Rust staticlib) | API, матчмейкинг, IAP, античит, лидерборды |
| **DB** | PostgreSQL + Redis | Профили, инвентарь, кэш, лидерборды |
| **Realtime** | WebSocket | Живые турниры, матчмейкинг, дуэли |

---

## 3. СТРУКТУРА МОНОРЕПО

```
gochya/
├── README.md
├── 00-MASTER-PROMPT.md
├── docs/                                ← вся документация
│   ├── 01-design/
│   ├── 02-mechanics/
│   ├── 03-architecture/
│   ├── 04-core/
│   ├── 05-security/
│   ├── 06-art/
│   └── 07-roadmap/
├── core/                                ← SHARED CORE (Rust)
│   ├── Cargo.toml
│   ├── src/
│   │   ├── lib.rs                       ← публичный API
│   │   ├── genome.rs                    ← Genome, наследование, мутации
│   │   ├── pet.rs                       ← Pet, потребности, эволюция
│   │   ├── combat.rs                    ← авто-баттлер, раунды
│   │   ├── technique.rs                 ← TechniqueCard, quality_score
│   │   ├── heart.rs                     ← HeartRateEvidence, validate_heart
│   │   ├── synergy.rs                   ← DailyActivitySnapshot, compute_vitality
│   │   ├── economy.rs                   ← валюты, цены, дроп-таблицы
│   │   ├── breeding.rs                  ← breed(), наследование
│   │   ├── matchmaking.rs               ← effective_power, tolerance
│   │   ├── rng.rs                       ← детерминированный Rng (seed)
│   │   ├── combat_ai.rs                 ← AI выбора карт (жадная эвристика)
│   │   └── serde_helpers.rs
│   ├── tests/                           ← unit + property + golden tests
│   └── ffi/                             ← биндинги
│       ├── swift/                       ← .xcframework (iOS + watchOS slices)
│       ├── jni/                         ← JNI для нативного Kotlin (Wear OS)
│       ├── dart_ffi/                    ← dart:ffi для Flutter
│       └── cgo/                         ← C-header для Go-сервера (cgo)
├── clients/
│   ├── watchos/                         ← Xcode project (Swift, SpriteKit)
│   ├── wearos/                          ← Android project (Kotlin, Filament)
│   └── companion/                       ← Flutter project (Dart)
├── server/                              ← Go backend (cgo)
│   ├── cmd/api/
│   ├── internal/
│   │   ├── auth/
│   │   ├── profile/
│   │   ├── inventory/
│   │   ├── matchmaking/
│   │   ├── combat/                      ← использует core через cgo (не WASM, см. §4)
│   │   ├── economy/
│   │   ├── iap/
│   │   ├── seasons/
│   │   └── anticheat/
│   ├── migrations/
│   └── tests/
└── tools/
    ├── ml-training/                     ← обучение классификатора ударов
    └── balance-sim/                     ← симуляции баланса
```

---

## 4. SHARED CORE — ТРАНСПОРТЫ

> **Решение аудита (S4, T1, T2):** зафиксировано **5 таргетов** компиляции (не 4), серверная интеграция — через **cgo** (нативная статическая линковка), WASM убран из основного пути.

Ядро компилируется в несколько форм:

| Таргет | Формат | Использовать |
|---|---|---|
| **iOS device** | static `.a` → `.xcframework` slice (`aarch64-apple-ios`) | Swift bridge через FFI |
| **watchOS device** | static `.a` → `.xcframework` slice (**`arm64_32-apple-watchos`**, Series 4+) | ⚠️ tier 3, nightly + `-Z build-std` |
| **iOS simulator** | `.xcframework` slice (`x86_64-apple-ios-simulator`) | разработка/тесты |
| **Android / Wear OS** | `.so` (`aarch64-linux-android`) | JNI для нативного Kotlin-клиента |
| **Server** | статическая `.a` (`x86_64-unknown-linux-gnu`) → **cgo** | серверная валидация |
| **Tests / CI** | native x86_64 | unit/property/golden tests |

> 🚨 **Важно (T1, уточнено аудитом):** watchOS-приложения для Apple Watch Series 4+
> используют ABI **`arm64_32`** — 64-битные регистры при 32-битных указателях.
> Нужный Rust-таргет — `arm64_32-apple-watchos`, и он **tier 3**: пребилда `std` нет,
> требуется nightly с `-Z build-std`. `aarch64-apple-watchos` — это симулятор
> и arm64-варианты, для устройства он не подходит.
>
> Практические следствия: CI для watchOS-слайса тянет nightly-toolchain;
> `usize` там 32-битный, поэтому в ядре **нельзя** полагаться на `usize == u64`
> при сериализации и FFI. Проверить спайком в Sprint 0 — это единственный таргет,
> способный заблокировать всю платформу.

> ⚠️ WASM (`wasm32-wasip1`, **не** `wasm32-unknown-unknown` — T2) оставлен как опция для будущего sandboxing, **не для MVP**. Альтернатива cgo на сервере — pure-Go `wazero` runtime, если cgo overhead станет узким местом.

Биндинги в `core/ffi/` генерируются автоматически (cbindgen для C-header → Swift/JNI/Kotlin).

---

## 5. ПОТОК ДАННЫХ (типичный сценарий: дуэль)

```
1. Игрок открывает PvP на часах
   → клиент запрашивает у сервера список доступных карт (из инвентаря)

2. Игрок собирает лоадаут (питомец + 5 карт + снаряжение)
   → клиент отправляет POST /matchmaking/queue {loadout}

3. Сервер находит соперника по effective_power
   → создаёт Match, генерирует matchSeed

4. Сервер вызывает core::combat::simulate(match, matchSeed)
   → ядро возвращает последовательность раундов (детерминированно)

5. Сервер сохраняет результат, обновляет ELO/лиги, выдаёт награды
   → отдаёт клиенту MatchResult (анимация + награды)

6. Клиент проигрывает анимацию по раундам
   → НЕ пересчитывает бой, только отображает
```

---

## 6. ПОТОК ДАННЫХ (запись удара)

```
1. Игрок заходит в Dojo, запускает pre-flight (пульс 8 сек)
   → локальная проверка contactConfidence

2. Запись 5–8 сек (акселерометр + гироскоп + пульс)
   → edge-ML классифицирует удар
   → вычисляются метрики (power, precision, combo, rhythm, HR_*)

3. Метрики + hash сигнала отправляются на сервер
   → POST /dojo/submit {metrics, signal_hash, heart_evidence}

4. Сервер:
   → anticheat-validate (см. ANTICHEAT.md)
   → core::technique::quality_score(metrics, heart_evidence)
   → определяет rarity, baseDamage, speed, effect
   → создаёт TechniqueCard, кладёт в инвентарь
   → rate-limit, replay-detection

5. Сервер возвращает TechniqueCard клиенту
   → клиент показывает карточку с анимацией
```

---

## 7. ПОТОК ДАННЫХ (активность → питомец)

```
1. Часы/телефон агрегируют дневную активность (нативные API)

2. Раз в час ИЛИ при открытии игры клиент синхронизируется:
   → POST /sync/activity {daily_snapshot, source_metadata}

3. Сервер:
   → anticheat-validate snapshot (дедупликация, типы активности)
   → goals := core::synergy::compute_goals(baseline)
   → total := core::synergy::compute_vitality(snapshot, goals, streak_days)
   → core::synergy::compute_stat_gains(snapshot, goals, genome, streak_days)
   → начисляет ТОЛЬКО прирост: delta = max(0, total − уже_начислено_за_день)
     (протокол идемпотентности — BACKEND.md §5, Activity Sync)

4. Сервер возвращает обновлённое состояние
   → клиент обновляет UI (кольца, питомца)
```

---

## 8. СИНХРОНИЗАЦИЯ СОСТОЯНИЯ

| Слой | Где живёт | Кто пишет |
|---|---|---|
| **Профиль игрока** | сервер (PostgreSQL) | сервер |
| **Питомец (состояние, статы)** | сервер + кэш на клиенте | сервер авторитет; клиент кэш |
| **Инвентарь** | сервер | сервер |
| **Сезоны, лиги, ELO** | сервер | сервер |
| **Сохранение сессии (offline)** | клиент (local) | клиент; синхронизируется при онлайне |

### Офлайн-режим
- На часах/телефоне игрок может кормить, чистить, тренироваться офлайн.
- Локальное состояние сохраняется с timestamp.
- При восстановлении соединения — **reconciliation**: сервер проверяет таймстампы и применяет накопленные изменения, отклоняя невозможные (больше действий, чем времени прошло).

### Разрешение конфликтов
- Сервер всегда авторитет. Если локальное состояние противоречит серверному — серверное побеждает.
- Документированные edge cases: смена часового пояса, перевод времени, двойной логин.

---

## 9. АВТОРИЗАЦИЯ И АККАУНТЫ

- **Sign in with Apple** (обязательно для watchOS/App Store).
- **Samsung Account** (для Galaxy Store).
- **Google Sign-In** (опционально для Android).
- Единый аккаунт GOCHYA: связывает стор-аккаунты в один профиль.
- Перенос прогресса между устройствами через аккаунт GOCHYA (не через стор).

---

## 10. CI/CD

| Пайплайн | Что делает |
|---|---|
| `core-ci` | cargo build/test/clippy под всеми таргетами; генерация биндингов |
| `watchos-ci` | xcodebuild для watchOS; unit-тесты |
| `wearos-ci` | Kotlin/Filament build для Wear OS; |
| `companion-ci` | flutter test/build для iOS + Android |
| `server-ci` | go test + линтеры + docker build |
| `integration` | e2e: клиент ↔ сервер, имитация дуэли, записи удара, синхронизации |
| `security` | SAST, dependency scan, проверка на секреты в коде |

---

## 11. PERFORMANCE BUDGETS (кратко, см. CLIENT_*.md)

| Метрика | Цель |
|---|---|
| Cold start watchOS | ≤ 1.5 сек |
| Cold start Wear OS (нативный Kotlin) | ≤ 2 сек |
| FPS активной игры (watchOS) | 60 |
| FPS активной игры (Wear OS) | 60 |
| FPS в AOD | 1 |
| Память active (watchOS) | ≤ 150 МБ |
| Память active (Wear OS) | ≤ 150 МБ |
| Память active (companion) | ≤ 400 МБ |
| Расход батареи в фоне | ≤ 2%/час |
| Расход батареи в Dojo (16 сек) | ≤ 0.5% |
| Размер APK на часах (Wear OS) | ≤ 20 МБ |
| Размер бандла watchOS | ≤ 50 МБ |
| Размер companion-app | ≤ 80 МБ |

---

## 12. СВЯЗАННЫЕ ДОКУМЕНТЫ

- `CLIENT_WATCHOS.md`, `CLIENT_WEAROS.md`, `CLIENT_COMPANION.md` — платформенные детали.
- `BACKEND.md` — сервер.
- `docs/04-core/CORE_SPEC.md` — контракт ядра.
- `docs/05-security/ANTICHEAT.md` — античит.
