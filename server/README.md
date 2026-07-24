# GOCHYA backend

Первый авторитетный серверный срез реализует Dojo:

- `POST /v1/dojo/preflight` выдаёт одноразовые `nonce` и attestation challenge
  с TTL 5 минут и стабильный для всего flow `traceId`;
- `POST /v1/dojo/submit` строго декодирует privacy-safe evidence, проверяет
  Ed25519-подпись, attestation policy, согласованность признаков и heart gate;
- повтор с тем же `Idempotency-Key` возвращает ту же карту, а повтор nonce,
  подмена подписанного payload и конфликт ключа отклоняются;
- `GET /v1/me/techniques` возвращает только карточки текущего игрока с
  cursor-pagination, не раскрывая attestation или sensor evidence;
- `POST /v1/me/techniques/equip` атомарно собирает лоадаут из пяти уникальных
  карт игрока, а `GET /v1/me/loadout` возвращает его текущую ревизию;
- `GET /v1/me`, `GET /v1/me/pets` и `GET /v1/me/pets/:id` возвращают
  авторитетный профиль и питомцев, а `POST .../:id/activate` меняет активного;
- характеристики карты вычисляются Rust Shared Core через статически
  слинкованный cgo ABI — формулы в Go не дублируются;
- deterministic combat доступен через тот же native Core как компактный
  `MatchV1 → MatchResultV1` snapshot с максимумом 20 раундов;
- `POST /v1/matchmaking/queue` создаёт idempotent casual match по двум
  серверным loadout, `GET /v1/match/:id` отдаёт сохранённый replay участнику,
  `GET /v1/me/matches/history` — его последние результаты, а
  `POST /v1/match/:id/confirm` один раз начисляет авторитетную награду и за
  первую UTC-победу выдаёт server-loot карту до Epic;
- `POST /v1/sync/activity` принимает нормализованный дневной health snapshot,
  вычисляет adaptive goals, vitality и stat gains через Rust Core и применяет
  только ещё не начисленную дельту; `GET /v1/me/activity/week` возвращает
  текущему игроку семь local-calendar days в хронологическом порядке, а
  `POST /v1/me/activity/reward` после 100 Vitality один раз выдаёт
  детерминированную server-loot карту до Rare;
- `POST /v1/breeding/breed` атомарно списывает 500 Koins, Love Crystal и
  выбранные catalysts, а Rust Core по server seed создаёт яйцо;
  `GET /v1/me/eggs` возвращает инкубирующиеся яйца, а
  `POST /v1/me/eggs/:id/hatch` конкурентно создаёт ровно одного питомца;
- `POST /v1/me/onboarding/age-gate` сохраняет только производную возрастную
  категорию, а `POST /v1/me/onboarding/starter-egg` один раз выдаёт выбранное
  Fire/Water/Earth яйцо с tutorial-инкубацией 5 секунд;
- активная стихия, владелец, ID и время создания назначаются сервером.

`internal/dojo.MemoryStore` — конкурентно-безопасная эталонная реализация для
unit-тестов. `PostgresStore` — production-реализация того же контракта поверх
pgxpool. Она блокирует строку игрока и nonce, после чего одной транзакцией
создаёт карту, audit/replay/idempotency-записи и потребляет nonce. В БД хранится
только SHA-256 nonce, не bearer-значение. Схема находится в `migrations/`;
`000000_base` создаёт обязательные `players`, `pets` и `technique_cards`, поэтому
весь реализованный vertical slice поднимается миграциями из пустой PostgreSQL.
Dojo `traceId` хранится рядом с nonce и audit row, передаётся через Go context в
attestation/Core/Store и возвращается в JSON и `X-Trace-ID`. Повтор с тем же
`Idempotency-Key` получает исходный trace вместе с исходной картой; отдельный
`X-Request-ID` остаётся идентификатором одной HTTP-попытки.

`internal/inventory` читает `technique_cards` в стабильном порядке
`created_at DESC, id DESC`. Cursor — канонический unpadded base64url, default
page size равен 20, максимум — 100. `id`, `ownerId` и `createdAt` в ответе
берутся из реляционных колонок, а не из дублирующего JSONB; timestamp
нормализуется в UTC независимо от timezone PostgreSQL-соединения. Миграция
`000005` добавляет покрывающий индекс для cursor query.

Экипировка требует `Idempotency-Key: <uuid>`, ровно пять уникальных `cardIds`
и `signatureIdx` от 0 до 4. Активный питомец выбирается сервером, поэтому клиент
не может экипировать чужого питомца или карту. `player_loadouts` из миграции
`000006` — источник истины с монотонной `revision`; флаги `is_equipped` и
`is_signature` на карточках обновляются в той же транзакции только как
проекция. Идентичный повтор в течение 24 часов возвращает сохранённый ответ, а
тот же ключ с другим телом получает `409 idempotency_conflict`.

`internal/profile` читает профиль и питомцев только в пределах JWT subject.
Питомцы упорядочены: активный первым, затем `created_at ASC, id ASC`.
Активация блокирует строку игрока, снимает прежний active flag, устанавливает
новый и в той же транзакции переносит существующий loadout на нового питомца с
увеличением `revision`. Повторная активация уже активного питомца не меняет
revision. Миграция `000007` добавляет индекс чтения и ограничения точной формы
`needs`/`stats`; Go decoder также fail-closed отклоняет неполные, расширенные и
выходящие за диапазоны Rust Core значения.

`internal/battle` реализует синхронный MVP-matcher для casual режима. Клиент
передаёт только `{ "mode": "casual" }` и UUID `Idempotency-Key`; loadout,
pet/card stats, seed и opponent выбирает сервер. PostgreSQL-транзакция
фиксирует обе loadout revision и snapshots, вызывает native Rust combat и
сохраняет replay до ответа. Временная advisory lock сериализует создание
матчей до внедрения Redis queue; повтор ключа в течение 24 часов не запускает
Core второй раз. Читать результат могут только оба участника. История также
ограничена участником, сортируется по `created_at DESC, id DESC`, принимает
`limit` от 1 до 100 (по умолчанию 20) и возвращает исход матча относительно
текущего игрока: `win`, `loss` или `draw`. Confirm не принимает outcome или
сумму от клиента: 30/20/10 Koins вычисляются из сохранённого результата.
Награждаются первые 10 матчей игрока за UTC-день; повторные и конкурентные
confirm возвращают одну запись из `match_confirmations`. Миграция `000009`
фиксирует награду, двухстороннюю ledger-запись и wallet projection в одной
транзакции. Миграция `000012` добавляет к первой casual-победе дня одну
детерминированную PvP-карту, сохраняя card ID и seed вместе с confirmation.

`internal/activity` нормализует allowlisted `sourceMetadata`, фиксирует SHA-256
fingerprint snapshot и сериализует sync блокировкой player row. День определяется
в сохранённой timezone игрока; текущий срез принимает только текущий local day.
Goals считаются Rust Core из среднего предыдущих 14 дней. Первый sync дня
фиксирует активного питомца, поэтому его смена в середине дня не переносит
оставшуюся дельту на другого питомца. Vitality никогда не отзывается при
коррекции snapshot вниз; stat gains, включая отрицательный FOC, приводятся к
новому дневному total с защитой pet stat от выхода ниже нуля. Миграция `000010`
хранит canonical snapshot, fingerprint, totals и фактически применённые gains.
Миграция `000011` связывает единственную дневную reward-карту с activity row и
хранит её seed; player lock сериализует конкурентные claims.

`internal/breeding` принимает только UUID родителей и имена опциональных
`mutation`/`hybrid` catalysts. Стадия, уровень, weakness, 24-часовой cooldown,
родословная до трёх поколений, wallet и item inventory читаются сервером.
Миграция `000013` создаёт `eggs`, `player_items`, двухсторонний item ledger и
неистекающий idempotency result: одинаковый UUID-ключ никогда не списывает
стоимость повторно. Геном и 4–24 часа инкубации вычисляет ABI 2.2.0; текущий
content gate выпускает только Fire/Water/Earth и Steam. Hatch блокирует player
и egg rows, поэтому конкурентные запросы возвращают одного сохранённого pet.

`internal/onboarding` реализует fail-closed вход в первый игровой цикл.
Age gate принимает дату рождения только для вычисления возрастной категории:
сама дата и её hash не сохраняются, в `onboarding_age_gate` остаются лишь
`under13 | 13plus`, версия политики, UUID идемпотентности и время фиксации.
`under13` получает `parental_consent_required`; текущий срез не имитирует
отсутствующий процесс проверки согласия. Для `13plus` сервер блокирует player
row, убеждается, что питомцев и яиц ещё нет, вызывает Core ABI 2.2.0 с
server-generated seed и атомарно сохраняет одно starter-яйцо. Частичный unique
index и сохранённый response делают конкурентные retries безопасными даже
после вылупления.

`JWTAuthenticator` проверяет Ed25519 access token, фиксированный алгоритм,
`kid`, issuer/audience, обязательные `exp`/`iat`/`jti`, `token_use=access` и
максимальный lifetime. `internal/auth` выпускает такие access tokens и
транзакционно вращает opaque refresh tokens; reuse отозванного токена отзывает
всю token family. В PostgreSQL хранится только SHA-256.
`PlayIntegrityVerifier` реализует Standard API policy:
request binding, freshness, package/version/certificate, `PLAY_RECOGNIZED`,
`LICENSED` и `MEETS_DEVICE_INTEGRITY`. HTTP decoder вызывает Google
`:decodeIntegrityToken`, а access token получает через Application Default
Credentials с OAuth scope `playintegrity`.

`GoogleVerifier` проверяет Google ID token официальным Go validator: подпись и
ротацию JWK с cache headers, точный allowlist `aud`, `exp`, оба допустимых
варианта `iss`, `iat` и непустой стабильный `sub`. Email не используется как
ключ и не сохраняется. `PostgresIdentityStore` атомарно создаёт игрока либо
возвращает существующего по `(auth_method, auth_subject)`, после чего auth
service выдаёт собственную session pair.

Native Sign in with Apple начинается с `POST /v1/auth/apple/preflight`: сервер
возвращает 256-bit nonce с TTL 5 минут, который клиент без преобразования
устанавливает в `ASAuthorizationAppleIDRequest.nonce`. `/v1/auth/apple` требует
тот же nonce и Apple identity token. `AppleVerifier` принимает только RS256,
выбирает RSA-ключ по `kid` из `https://appleid.apple.com/auth/keys`, проверяет
точные `iss`/allowlist `aud`, `exp`, `iat`, nonce и стабильный `sub`. Кэш JWK
обновляется при неизвестном `kid`, а частые miss ограничены. Nonce атомарно
потребляется один раз; в PostgreSQL хранится только его SHA-256. Email из токена
не является ключом и не сохраняется.

Samsung Account использует обнаруживаемый OIDC authorization-code flow, а не
доверяет access token, принесённому клиентом. `/v1/auth/samsung/preflight`
принимает allowlisted `redirectUri`, создаёт одноразовые state/nonce и PKCE
S256, сохраняет только SHA-256 state и его binding. Клиент открывает полученный
`authorizationUrl`, а code из callback отправляет вместе с исходными
state/nonce/codeVerifier в `/v1/auth/samsung`. Backend атомарно потребляет state,
обменивает code на token endpoint с confidential client secret и проверяет
RS256 ID token по Samsung JWK: точные issuer/client audience, `exp`, `iat`,
nonce, `azp` для multi-audience и стабильный `sub`. Provider access/refresh
tokens не сохраняются.

Samsung client ID, secret и redirect URIs выдаются при регистрации OIDC-клиента.
Перед production rollout остаётся пройти partner/on-device gate реальными
credentials и подтвердить поддержку PKCE Samsung tenant'ом; небезопасного
fallback к непроверенному access token нет.

Wear OS key enrollment реализован отдельным challenge-response:
`POST /v1/devices/preflight` связывает одноразовый 256-bit challenge с
аутентифицированным player, `deviceId`, `platform` и allowlisted `appBuild`.
`POST /v1/devices/register` принимает 32-byte Ed25519 public key,
proof-of-possession подпись и Play Integrity Standard token. Подпись и
`requestHash` покрывают один канонический registration payload, поэтому
произвольный ключ нельзя подложить отдельно от attested запроса. PostgreSQL
хранит только SHA-256 challenge; row lock игрока сериализует одновременную
первичную регистрацию, а consume challenge и insert устройства выполняются
одной транзакцией. Повтор того же активного ключа с новым challenge идемпотентен,
но замена ключа или включение отключённого устройства не происходят неявно.
watchOS намеренно возвращает `platform_unsupported`, пока не реализован
отдельный App Attest enrollment.

Пока verifier явно не передан в `ServiceConfig`, приложение должно использовать
`RejectingAttestationVerifier`: он намеренно закрывает выдачу карт.

Wear OS передаёт `attestation.provider = "play_integrity_standard"`. Standard
API `requestHash` вычисляется без padding:

```text
canonical = CanonicalPayload(submit без attestation и payloadSignature)
requestHash = base64url(SHA-256(
    "gochya-dojo-play-integrity-v1" || 0x00 ||
    challenge || 0x00 || canonical
))
```

`appBuild` для Wear OS — десятичная строка Play `versionCode`. Временная
недоступность Google/ADC возвращает `503 attestation_unavailable`; плохой verdict
возвращает `401 attestation_invalid`. Одинаковые одновременные decode-запросы
внутри процесса объединяются, а их проверенный результат на две минуты
кэшируется по SHA-256 от encrypted token и `requestHash`; сам token в кэше не
хранится. Временные ошибки не кэшируются и допускают retry.

`HeaderAuthenticator` доверяет `X-Player-ID` и предназначен только для тестов и
закрытого staging. HTTP handler требует явной передачи authenticator, поэтому
этот режим нельзя включить неявно.

## Production API

`cmd/api` собирает production dependency graph без тестовых fallback:
PostgreSQL Store, Ed25519 JWT issuance/verification, refresh rotation,
Google ADC, Play Integrity Standard и нативный Rust Core. При старте процесс
проверяет соединение и наличие обязательных таблиц в БД, а пробным ABI-вызовом
убеждается, что Core действительно слинкован. Миграции должны быть применены
отдельным deployment job до запуска API; процесс с неполной схемой не стартует.

Обязательная конфигурация:

| Переменная | Формат |
|---|---|
| `GOCHYA_DATABASE_URL` | PostgreSQL connection string |
| `GOCHYA_JWT_ISSUER` | точный JWT `iss` |
| `GOCHYA_JWT_AUDIENCE` | обязательный JWT `aud` |
| `GOCHYA_JWT_PUBLIC_KEYS_JSON` | JSON `kid →` 32-byte unpadded base64url Ed25519 public key |
| `GOCHYA_JWT_SIGNING_KEY_ID` | активный `kid`, присутствующий в public key map |
| `GOCHYA_JWT_SIGNING_PRIVATE_KEY` | 64-byte unpadded base64url Ed25519 private key из secret manager |
| `GOCHYA_GOOGLE_CLIENT_IDS` | CSV разрешённых Google OAuth client IDs (`aud`) |
| `GOCHYA_APPLE_CLIENT_IDS` | CSV нативных Apple App IDs / bundle IDs (`aud`) |
| `GOCHYA_SAMSUNG_OIDC_CLIENT_ID` | Samsung Account OIDC client ID |
| `GOCHYA_SAMSUNG_OIDC_CLIENT_SECRET` | Samsung OIDC client secret из secret manager |
| `GOCHYA_SAMSUNG_REDIRECT_URIS` | CSV точных зарегистрированных HTTPS redirect URIs |
| `GOCHYA_PLAY_PACKAGE_NAME` | Android package name |
| `GOCHYA_PLAY_CERTIFICATE_SHA256_DIGESTS` | CSV разрешённых Play certificate digests |
| `GOCHYA_ALLOWED_APP_BUILDS` | CSV разрешённых `versionCode` |
| `GOCHYA_ALLOWED_CLASSIFIER_VERSIONS` | CSV разрешённых classifier versions |

`GOCHYA_HTTP_ADDRESS` по умолчанию равен `:8080`.
`GOCHYA_PLAY_REQUIRED_DEVICE_VERDICTS` по умолчанию требует
`MEETS_DEVICE_INTEGRITY`. Флаги `GOCHYA_PLAY_ALLOW_UNLICENSED` и
`GOCHYA_PLAY_ALLOW_TEST_RESPONSES` по умолчанию `false` и предназначены только
для явно изолированного staging.

Сборка после Rust Core:

```bash
cargo build --release -p gochya-core
cd server
CGO_ENABLED=1 go build -tags gochya_core -o ../target/gochya-api ./cmd/api
```

Процесс обслуживает API и probes:

```text
POST /v1/dojo/preflight
POST /v1/dojo/submit
GET  /v1/me
GET  /v1/me/pets
GET  /v1/me/pets/:id
POST /v1/me/pets/:id/activate
POST /v1/matchmaking/queue
POST /v1/sync/activity
GET  /v1/me/activity/week
POST /v1/me/activity/reward
POST /v1/breeding/breed
POST /v1/me/onboarding/age-gate
POST /v1/me/onboarding/starter-egg
GET  /v1/me/eggs
POST /v1/me/eggs/:id/hatch
GET  /v1/match/:id
POST /v1/match/:id/confirm
GET  /v1/me/matches/history?limit=
GET  /v1/me/techniques?limit=&cursor=
POST /v1/me/techniques/equip
GET  /v1/me/loadout
POST /v1/devices/preflight
POST /v1/devices/register
POST /v1/auth/google
POST /v1/auth/apple/preflight
POST /v1/auth/apple
POST /v1/auth/samsung/preflight
POST /v1/auth/samsung
POST /v1/auth/refresh
POST /v1/auth/logout
GET  /health/live
GET  /health/ready
```

Readiness проверяет PostgreSQL с отдельным таймаутом; HTTP-сервер задаёт
ограничения заголовков и read/write/idle timeouts и корректно завершает
активные запросы при `SIGTERM`/`SIGINT`.

## Проверка

Из корня репозитория:

```bash
bash tools/check-server.sh
```

Команда собирает `libgochya_core.a`, проверяет форматирование и `go vet`, затем
запускает Go-тесты с race detector и build tag `gochya_core`.

PostgreSQL integration tests запускаются автоматически, если определён
`GOCHYA_TEST_DATABASE_URL`. Они создают отдельные временные schema, применяют
полную цепочку миграций из пустой БД и проверяют конкурентные submit, auth
consume, device enrollment, идемпотентную экипировку и атомарную активацию
питомца:

```bash
GOCHYA_TEST_DATABASE_URL='postgres://user:pass@127.0.0.1:5432/gochya_test?sslmode=disable' \
  bash tools/check-server.sh
```

Обычные unit-тесты можно запускать без нативной библиотеки:

```bash
cd server
go test ./...
```

Без build tag нативный `corebridge.NativeEngine` возвращает
`corebridge.ErrUnavailable`, а не воспроизводит игровые формулы.
