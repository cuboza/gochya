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
