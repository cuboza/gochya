# CLAUDE.md

Инструкции для AI-агента, работающего в этом репозитории. Полный контекст —
[`00-MASTER-PROMPT.md`](./00-MASTER-PROMPT.md); здесь только то, что нужно
каждый раз.

## Что это

GOCHYA — виртуальный питомец: Rust Shared Core, Go-бэкенд поверх него через cgo
и Flutter-клиент телефона. Клиенты часов (`clients/wearos/`, `clients/watchos/`)
ещё не начаты.

| Каталог | Что внутри |
|---|---|
| [`core/`](./core/) | Rust Shared Core + versioned C ABI (`core/ffi/gochya_core.h`) |
| [`server/`](./server/) | Go API, PostgreSQL, cgo-мост в Core |
| [`clients/companion/`](./clients/companion/) | Flutter-клиент Android/iOS |
| [`docs/`](./docs/) | Спецификации — источник истины для поведения |
| [`tools/`](./tools/) | Локальные проверки и симуляторы баланса |

## Ненарушимые правила

1. **Формулы живут только в Core.** Сервер и клиенты вызывают ядро, но не
   повторяют его арифметику. Новая формула — сначала в `CORE_FORMULAS.md`, потом
   в Rust, потом вызов.
2. **Сервер — авторитет.** Бой, экономика, геном, награды считаются на сервере.
   Клиент отправляет намерение и отображает канонический ответ. Единственное
   исключение — `technique_type` (см. `ANTICHEAT.md §3.5`).
3. **Сырые данные сенсоров не покидают устройство.** На сервер уходят только
   производные агрегаты.
4. **Идемпотентность обязательна для любой мутации.** Ключ переиспользуется при
   повторе: неопределённый исход не должен списывать ресурсы дважды.
5. **Документация — часть задачи.** Изменил поведение — обнови соответствующий
   `.md`.

## Локальные проверки

```bash
bash tools/check-core.sh          # Rust: fmt, clippy, тесты, C ABI smoke
bash tools/check-server.sh        # Go: gofmt, vet, тесты с cgo (нужен PostgreSQL)
node tools/check-markdown-links.mjs

bash tools/build-core-host.sh     # FFI-тесты клиента идут против реального ядра
cd clients/companion
dart format lib test
dart analyze lib test             # флаг --no-pub у flutter analyze падает на info
flutter test --no-pub
```

`GOCHYA_TEST_DATABASE_URL` включает интеграционные тесты сервера; без неё они
пропускаются. Поднять базу для них:

```bash
docker run -d --name gochya-pg -p 5432:5432 \
  -e POSTGRES_DB=gochya_test -e POSTGRES_USER=gochya -e POSTGRES_PASSWORD=gochya \
  postgres:16-alpine
export GOCHYA_TEST_DATABASE_URL='postgres://gochya:gochya@127.0.0.1:5432/gochya_test?sslmode=disable'
```

С ней проходит весь набор, включая транзакционные тесты хранилищ — их стоит
гонять при любой правке SQL, иначе они молча пропускаются и ошибка уезжает в CI.

## Конвенции Flutter-клиента

- Слои: `core/models` (строгий декодер JSON) → `core/network` (типизированный
  boundary) → `features/<domain>/<domain>_repository.dart` (Riverpod-провайдеры)
  → `features/<domain>/<domain>_screen.dart`.
- Декодеры **строгие**: невозможное значение — это `FormatException`, а не
  тихое клампирование. Помощники живут в `core/models/profile_models.dart`.
- Каждый авторизованный запрос идёт через `AuthenticatedRequestRunner`, который
  делает single-flight refresh и fail-closed при неопределённом исходе.
- UI-текст — русский; идентификаторы, комментарии и docstring — английский.
- Демо-режим (`lib/dev/demo_mode.dart`) подменяет репозитории: добавил новый
  репозиторий — добавь и его демо-реализацию.
- Мост в Core (`core/ffi/`) не содержит арифметики: только marshalling и перевод
  не-`Ok` статуса в `CoreException`. Ядро отвергло вход — пробрасывай отказ, не
  клампи. Его тесты идут против реальной библиотеки.

## Что ещё не сделано

Sprint 0 Gates 1–4 (`docs/07-roadmap/SPRINT0_GATES.md`) требуют реальных
устройств и не закрываются кодом. Клиенты часов и ingestion health-данных
(Health Connect / HealthKit) — отдельные срезы; клиент не должен выдумывать
показатели активности вместо них.
