# GOCHYA backend

Первый авторитетный серверный срез реализует Dojo:

- `POST /v1/dojo/preflight` выдаёт одноразовые `nonce` и attestation challenge
  с TTL 5 минут;
- `POST /v1/dojo/submit` строго декодирует privacy-safe evidence, проверяет
  Ed25519-подпись, attestation policy, согласованность признаков и heart gate;
- повтор с тем же `Idempotency-Key` возвращает ту же карту, а повтор nonce,
  подмена подписанного payload и конфликт ключа отклоняются;
- характеристики карты вычисляются Rust Shared Core через статически
  слинкованный cgo ABI — формулы в Go не дублируются;
- активная стихия, владелец, ID и время создания назначаются сервером.

`internal/dojo.MemoryStore` — конкурентно-безопасная эталонная реализация для
unit-тестов. `PostgresStore` — production-реализация того же контракта поверх
pgxpool. Она блокирует строку игрока и nonce, после чего одной транзакцией
создаёт карту, audit/replay/idempotency-записи и потребляет nonce. В БД хранится
только SHA-256 nonce, не bearer-значение. Схема находится в `migrations/`.

`JWTAuthenticator` проверяет Ed25519 access token, фиксированный алгоритм,
`kid`, issuer/audience, обязательные `exp`/`iat`/`jti`, `token_use=access` и
максимальный lifetime. `internal/auth` выпускает такие access tokens и
транзакционно вращает opaque refresh tokens; reuse отозванного токена отзывает
всю token family. В PostgreSQL хранится только SHA-256.
`PlayIntegrityVerifier` реализует Standard API policy:
request binding, freshness, package/version/certificate, `PLAY_RECOGNIZED`,
`LICENSED` и `MEETS_DEVICE_INTEGRITY`. HTTP decoder вызывает Google
`:decodeIntegrityToken`, а access token получает через Application Default
Credentials с OAuth scope `playintegrity`. Проверка credentials внешних
OAuth-провайдеров остаётся отдельным следующим модулем.

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

PostgreSQL integration test запускается автоматически, если определён
`GOCHYA_TEST_DATABASE_URL`. Он создаёт отдельную временную schema, применяет
миграцию и проверяет конкурентный submit:

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
