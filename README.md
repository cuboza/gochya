# GOCHYA — Полная документация проекта

> **Виртуальный питомец с полноценной игрой на телефоне** (iOS/Android) **и премиум-опытом на часах** (Galaxy Watch / Apple Watch), с записью реальных ударов, бридингом, синхронизацией с активностью владельца и PvP-чемпионатами. Телефон проходит игру целиком; часы добавляют запись ударов (Dojo) и трекинг с запястья.

Эта документация — **исходный бриф для AI-разработки**. Каждый файл самодостаточен, но часть спецификаций кросс-ссылается. Прочтите `00-MASTER-PROMPT.md` первым — он задаёт контекст и порядок работы.

## Статус реализации

Sprint 0 начат: в [`core/`](./core/) находится первый рабочий срез Rust Shared
Core с heart gate, расчётом Technique Card, vitality, детерминированным casual
combat, golden/property-тестами и versioned C ABI. Combat теперь также доступен
серверу через компактный `MatchV1 → MatchResultV1` cgo-контракт без дублирования
формул. В [`server/`](./server/)
реализован первый Go-срез Dojo: preflight, подписанный submit, attestation
boundary, replay/idempotency/rate-limit, транзакционный PostgreSQL Store и
настоящий cgo-вызов Rust Core. HTTP-boundary поддерживает проверяемые
Ed25519 JWT access tokens с ротацией ключей через `kid` и Play Integrity
Standard verdict, привязанный к каноническому Dojo payload.
Auth-модуль выпускает access tokens и выполняет атомарную refresh rotation с
token-family reuse detection; Google ID token exchange создаёт или находит
игрока по проверенному provider `sub`. Native Sign in with Apple использует
одноразовый server nonce, проверяет RS256 ID token по ротируемым Apple JWK и
также связывает аккаунт только со стабильным `sub`, а не с email.
Samsung Account подключён через OIDC authorization-code flow: backend выдаёт
state/nonce/PKCE challenge, сам обменивает одноразовый code с confidential
client credentials и проверяет Samsung RS256 ID token.
Flutter-клиент теперь использует этот refresh-контракт: параллельные `401`
объединяются в одну rotation, новая пара сохраняется одним защищённым документом,
а исходный запрос повторяется ровно один раз. Потерянный refresh-ответ,
повторный `401` или ошибка записи завершают локальную сессию fail-closed без
опасного повторного использования одноразового refresh token.
Android-клиент также выполняет нативный Google Sign-In и передаёт backend только
Google ID token вместе со стабильным installation ID. GOCHYA-сессия появляется
лишь после успешного `POST /v1/auth/google`; без Web OAuth client ID кнопка входа
скрыта fail-closed, ручного ввода bearer token нет.
iOS-клиент выполняет нативный Sign in with Apple: получает одноразовый server
nonce, передаёт его AuthenticationServices без преобразования и обменивает
Apple identity token только через backend. Capability включён в Xcode project,
а недоступный провайдер или невалидный nonce не создают локальную сессию.
Явный выход телефона теперь best-effort отзывает server-side refresh family и
всегда очищает локальные credentials/очередь даже офлайн. Session generation
блокирует позднюю refresh-ротацию, чтобы она не восстановила сессию после logout
и не повторила intent старого аккаунта с токеном нового входа.
Wear OS device enrollment требует access token, одноразовый server challenge,
proof-of-possession нового Ed25519-ключа и Play Integrity verdict, привязанный
к тому же регистрационному payload. Challenge хранится только как SHA-256 и
потребляется в одной PostgreSQL-транзакции с регистрацией ключа.
Dojo flow получает стабильный `traceId`, который связывает preflight,
attestation, Rust Core, PostgreSQL audit и idempotent HTTP-ответ.
Созданные Technique Cards доступны текущему игроку через authenticated
cursor-paginated inventory API; приватный Dojo evidence в него не попадает.
Игрок может атомарно экипировать пять принадлежащих ему карт, выбрать одну
signature-позицию и читать текущий server-authoritative loadout с монотонной
ревизией и идемпотентными повторами.
Authenticated profile API возвращает только питомцев текущего игрока; bounded
lineage-граф раскрывает до трёх поколений без owner/needs/stats предков. Смена
активного питомца транзакционно синхронизирует существующий loadout и не
увеличивает его revision при повторе.
Casual match теперь целиком считается native Rust Core по двум авторитетным
server-side loadout; PostgreSQL сохраняет seed, обе revision и replay, а
idempotent retry не создаёт второй бой. Первая casual-победа UTC-дня выдаёт
детерминированную PvP-карту до Epic и возвращает её при повторном confirm.
Телефонный игрок может получить первую server-authoritative Technique Card без
Dojo: после 100 дневной Vitality activity reward детерминированно рассчитывается
Rust Core из сохранённого seed и конкурентно выдаётся ровно один раз за локальный
день.
Бридинг теперь также проходит целиком через сервер: два здоровых взрослых
питомца, 500 Koins и Love Crystal атомарно превращаются в детерминированное
яйцо Rust Core; mutation/hybrid catalysts расходуются из item ledger, а
конкурентное вылупление создаёт ровно одного питомца.
Offline care пересчитывает потребности, Sleep и Weakness через ABI 2.3, а
идемпотентный command reconcile атомарно расходует предметы. Новый
server-authoritative Koins-shop публикует согласованные care/breeding SKU,
проводит покупку через currency/item ledgers и возвращает приватный inventory.
Read-only `server/cmd/ledger-audit` даёт cron/deployment gate: в одном
PostgreSQL snapshot сверяет Koins, дневную Vitality и item projections с
ledger, а также проверяет нулевую сумму обеих сторон каждой записи.
`server/cmd/api` собирает эти компоненты в fail-closed production-процесс с
PostgreSQL readiness probe, HTTP timeouts и graceful shutdown.
Полная текущая цепочка PostgreSQL migrations теперь стартует из пустой schema,
без ручного создания базовых таблиц.
В [`clients/companion/`](./clients/companion/) создан исполняемый Flutter-клиент
Android/iOS: защищённая локальная сессия, Material app shell, типизированные
profile/pets/lineage contracts, главная активного питомца и bounded lineage UI.
Новый игрок проходит обязательный privacy-safe age gate, выбирает одно из трёх
server-authoritative starter-яиц, возобновляет инкубацию после перезапуска и
вылупляет первого питомца. Категория `under13` блокируется до появления
проверяемого parental-consent flow, без клиентского обхода.
Активного питомца уже можно кормить яблоком, чистить, развлекать и укладывать
спать: Flutter-клиент отправляет care-команды в `POST /v1/sync/commands` с
`If-Match`, постоянным локальным `deviceId` и полным идемпотентным intent.
Account-bound encrypted journal сохраняет до 100 команд перед отправкой,
восстанавливает тот же payload после перезапуска и последовательно отправляет
batch одного питомца. Терминально подтверждённые команды удаляются, `RETRYABLE`
остаётся в очереди, а logout/смена аккаунта очищает старый журнал. Итоговое
состояние всегда перечитывается с сервера.
Вкладка магазина читает авторитетные каталог, Koins и приватный инвентарь,
отправляет только `itemId`/`quantity` с UUID idempotency key и применяет
канонический результат покупки. Потерянный ответ блокирует новые покупки до
повторной синхронизации, поэтому клиент не создаёт второй денежный intent.
Телефон закрывает и остальную вертикаль MVP: собирает server-authoritative
лоадаут из пяти карт с одной signature-позицией, ставит casual-бой, отображает
серверный replay и идемпотентно забирает награду с первой картой дня; скрещивает
двух здоровых взрослых родителей с катализаторами и вылупляет яйцо; показывает
неделю Vitality и забирает карту за 100 Vitality. Ни одна из формул при этом не
дублируется в Dart, а повтор любой мутации переиспользует тот же idempotency key.
Ingestion health-данных (Health Connect / HealthKit) сознательно оставлен
отдельным срезом: клиент не сочиняет показатели активности.
Телефон также получил визуальный слой: канонический арт стихий MVP на главном
экране, отдельный дизайн Technique Card под каждый удар с рамкой по шкале
редкости, описанием и легендой, idle-анимацию питомца и покадровое проигрывание
серверного replay в бою с уроном, эффектами и HP-барами. Replay теперь несёт
виды обеих сторон, поэтому игрок видит настоящего соперника. Существа
анимируются скиннингом меша по костям — двигаются голова, хвост и конечности, —
с замахом, проводкой и докрутом по `ART_BIBLE.md` §9.1; разыгранная карта раунда
влетает с переворотом и свечением по редкости.
Его CI проверяет форматирование, analyzer, widget/API tests и нативные Android/iOS
сборки, а отдельный documentation job проверяет локальные Markdown-ссылки.
Реальный Wear OS device gate и watchOS App Attest gate ещё не выполнялись.

Локальная проверка:

```bash
bash tools/check-core.sh
bash tools/check-server.sh
cd clients/companion
flutter analyze
flutter test
```

---

## 📚 Структура документации

### 🎯 Стартовые точки (читать в порядке)

| # | Файл | Назначение |
|---|---|---|
| 🚀 | [`00-MASTER-PROMPT.md`](./00-MASTER-PROMPT.md) | **Главная инструкция для AI-агента**: контекст, принципы, как пользоваться документацией, режимы работы |
| 📋 | [`01-design/EXECUTIVE_SUMMARY.md`](./docs/01-design/EXECUTIVE_SUMMARY.md) | Краткое описание продукта: цель, УТП, платформы, метрики успеха |

### 🎮 Гейм-дизайн (`docs/01-design/`)

| Файл | Содержание |
|---|---|
| `EXECUTIVE_SUMMARY.md` | Видение, USP, KPI, целевая аудитория |
| `GDD.md` | Основной Game Design Document: ядерные петли, уход, эволюция, PvP, экономика, монетизация |
| `GAME_LOOP.md` | Детализация core loop, retention-петли, дневные/недельные/сезонные циклы |

### ⚙️ Механики (`docs/02-mechanics/`)

| Файл | Механика |
|---|---|
| `MECHANIC_COMBAT_RECORDING.md` | **Запись ударов с акселерометра** → Technique Cards |
| `MECHANIC_BREEDING.md` | **Бридинг**: геном, наследование, мутации, гибриды |
| `MECHANIC_SYNERGY.md` | **Симбиоз**: реальная активность владельца растит питомца |
| `MECHANIC_HEART_GATE.md` | **Пульс-античит** для валидации записи ударов |
| `MECHANIC_ML_CLASSIFIER.md` | **ML-классификатор ударов**: DTW, шаблоны, план сбора датасета |

### 🏗 Архитектура (`docs/03-architecture/`)

| Файл | Содержание |
|---|---|
| `ARCHITECTURE.md` | Общая архитектура «общее ядро + нативные оболочки», диаграммы |
| `CLIENT_WATCHOS.md` | Нативный клиент на Apple Watch (Swift/SwiftUI/**SpriteKit**) |
| `CLIENT_WEAROS.md` | **Нативный** клиент на Galaxy Watch / Wear OS (**Kotlin + Filament**) |
| `CLIENT_COMPANION.md` | Flutter companion-приложение для телефона |
| `BACKEND.md` | Сервер, БД, матчмейкинг, IAP-валидация, античит |
| `OFFLINE_SYNC.md` | Идемпотентный журнал офлайн-команд и разрешение конфликтов |
| `HEALTH_DATA_CONTRACT.md` | Source-of-truth, provenance и дедупликация health-данных |

### 🧩 Shared Core (`docs/04-core/`)

| Файл | Содержание |
|---|---|
| `CORE_SPEC.md` | Контракт ядра: типы, public API, сериализация, инварианты |
| `CORE_FORMULAS.md` | Все формулы собраны в одном месте (баланс, урон, мутации, vitality) |
| `CORE_ABI.md` | Стабильный C ABI для Go, Swift, Kotlin/JNI и Dart FFI |

### 🔒 Безопасность и баланс (`docs/05-security/`)

| Файл | Содержание |
|---|---|
| `ANTICHEAT.md` | Многослойный античит: heart gate, replay-detection, клиентская энтропия, вероятностный аудит (privacy-first) |
| `BALANCE.md` | Числа баланса, кривые прогрессии, дроп-таблицы гачи |

### 🎨 Арт и UX (`docs/06-art/`)

| Файл | Содержание |
|---|---|
| `ART_BIBLE.md` | Визуальный стиль, палитра, ассеты, пайплайн под 3 рантайма |
| `UX_UI.md` | UX для круглого и прямоугольного экранов, companion-UI |
| `artifacts/GOCHYA_UI_CONCEPT_V1.md` | Визуальный mockup телефона, Wear OS и Fire-персонажа |
| `artifacts/GOCHYA_CONCEPT_PACK_V2.md` | Разные виды стихий и полный набор UI concept sheets |
| `artifacts/ELEMENTAL_CREATURES_MATURE_V2.md` | Промежуточный, более фэнтезийный character direction (устарел) |
| `artifacts/ELEMENTAL_CREATURES_GROUNDED_V3.md` | Канонический grounded character direction для MVP-стихий |
| `prototypes/animated-ui/` | Интерактивный макет телефона и Wear OS с анимированными персонажами |

### 🗺 План (`docs/07-roadmap/`)

| Файл | Содержание |
|---|---|
| `MVP.md` | Скоуп вертикального среза, критерии готовности |
| `CONTENT_MANIFEST.md` | Единый объём существ, ударов, предметов и ассетов по фазам |
| `ROADMAP.md` | Alpha → Beta → Soft launch → Global, метрики-gate |
| `RISKS.md` | Риски и стратегии снижения |
| `AUDIT_REMEDIATION_PLAN.md` | Рабочий план устранения находок аудита и Sprint 0 gate'ов |
| `SPRINT0_GATES.md` | Проверяемые чек-листы, evidence и fail-actions для технологических gate'ов |

---

## 🧭 Кому какой файл читать

- **AI-разработчик (полный цикл)** → `00-MASTER-PROMPT.md` → затем по разделам.
- **Backend-инженер** → `ARCHITECTURE.md` + `BACKEND.md` + `CORE_SPEC.md` + `ANTICHEAT.md`.
- **Engineer Shared Core** → `CORE_SPEC.md` + `CORE_FORMULAS.md` + `BALANCE.md`.
- **watchOS-инженер** → `CLIENT_WATCHOS.md` + `MECHANIC_COMBAT_RECORDING.md` + `MECHANIC_HEART_GATE.md` + `CORE_SPEC.md`.
- **Wear OS / Kotlin-инженер** → `CLIENT_WEAROS.md` + механики + `CORE_SPEC.md`.
- **Flutter / companion** → `CLIENT_COMPANION.md` + `GDD.md` (магазин, турниры, родословная).
- **Геймдизайнер** → все `docs/01-design/` + `docs/02-mechanics/` + `BALANCE.md`.
- **Художник/арт-лид** → `ART_BIBLE.md` + `UX_UI.md` + `EXECUTIVE_SUMMARY.md`.

---

## ⚡ Быстрые принципы проекта

1. **Общее ядро — source of truth.** Вся игровая логика детерминирована и одинакова на всех платформах.
2. **Сервер — авторитет.** Бой и экономика считаются на сервере. Клиент только отображает.
3. **Батарея важнее красоты.** AOD ≤1 FPS, никаких фоновых опросов сенсоров игрой.
4. **Активность — это бонус, не наказание.** Без спорта игра проигрывается на ~60%, но не блокируется.
5. **Этичная монетизация.** PvP-баланс по «эффективной силе», опубликованные дропы гачи.
6. **Реальные движения = геймплей.** Запись ударов и трекинг активности — USP, не побочная фича.

---

*Версия документации: 1.0 · Дата: 2026-07-20*
