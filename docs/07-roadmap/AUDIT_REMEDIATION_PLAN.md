# План исправлений по итогам аудита

> Рабочий план закрытия рисков перед production. Статусы обновляются вместе с документацией и реализацией.

## Цель

Довести проект от набора спецификаций до проверенного вертикального прототипа, после которого можно утвердить MVP-скоуп, команду и сроки.

## Фаза 1 — Fast wins

- [x] Зафиксировать: полноценный watchOS-клиент относится к Beta, в MVP входит только Gate 2 spike.
- [x] Зафиксировать контент: три визуально разных стартовых вида и отдельный гибридный вид Steam.
- [x] Заменить Google Fit в целевой Android-архитектуре на Health Connect.
- [x] Исправить ссылку на Samsung partner approval: `RISKS.md` R16.
- [x] Унифицировать технологический стек: Shared Core — Rust, backend — Go.
- [x] Исправить watchOS Rust target на `arm64_32-apple-watchos`.
- [x] Добавить автоматический локальный Markdown link checker без внешних зависимостей.

## Фаза 2 — MVP и контент

- [x] Разделить обязательный vertical slice и функции Alpha/Beta.
- [x] Перенести ranked, сезоны, Battle Pass, друзей и IAP из MVP в Alpha/Beta.
- [x] Создать content manifest: модели, варианты, анимации, карты, предметы и экраны по фазам.
- [x] Добавить измеримые acceptance criteria для каждого сценария vertical slice.
- [x] Пересчитать provisional команду и сроки после фиксации скоупа; обязательная повторная оценка — после Sprint 0.

**Gate:** один однозначный MVP backlog без функций, одновременно указанных в MVP и Beta.

## Фаза 3 — Security и Dojo

- [x] Описать threat model: replay, MITM, modified client, root/jailbreak и bot farm.
- [x] Спроектировать Play Integrity для Android и App Attest/DeviceCheck для iOS.
- [x] Привязать server nonce, payload, версию клиента и device attestation.
- [x] Определить privacy-safe evidence для Dojo вместо полностью доверенных итоговых чисел.
- [x] Запретить выдачу карты без допустимого attestation; временный сбой переводит запись в pending/retry.

**Gate:** сервер различает валидный официальный клиент, replay и произвольный payload модифицированного клиента; остаточные ограничения явно приняты.

## Фаза 4 — Контракты реализации

- [x] Создать `CORE_ABI.md`: `repr(C)`, enum values, opaque handles, ownership, buffers, errors, threading и ABI version.
- [x] Добавить генерируемый `gochya_core.h` и нативный C ABI smoke test для первого surface (cross-language consumers остаются Gate 2/5).
- [x] Описать offline command protocol: `operation_id`, `device_id`, `base_revision`, preconditions и reconciliation.
- [x] Зафиксировать source-of-truth и дедупликацию HealthKit/Health Connect/Samsung Health.
- [x] Зафиксировать schema/version migration для JSONB-сущностей Shared Core.

**Gate:** один и тот же golden fixture проходит через Rust, Go, Flutter и Wear OS без расхождений ABI и сериализации.

## Фаза 5 — Sprint 0 и вертикальный прототип

- [ ] Выполнить Gate 1: Filament + сенсоры + performance budget на Galaxy Watch 6/7 (`SPRINT0_GATES.md`).
- [ ] Выполнить Gate 2: Rust FFI + SpriteKit spike на реальном Apple Watch (`SPRINT0_GATES.md`).
- [ ] Выполнить Gate 3: Samsung partner request и измерения HR frequency/latency (`SPRINT0_GATES.md`).
- [ ] Выполнить Gate 4: старт и валидация датасета классификатора (`SPRINT0_GATES.md`).
- [ ] Собрать end-to-end путь: Wear OS → API → Rust Core → PostgreSQL → Flutter.
- [ ] Настроить CI для Rust, Go, Flutter, Wear OS, ABI и документации.
  Rust/Go/Core и Flutter analyzer/tests/Android/iOS build уже автоматизированы;
  отдельные Wear OS и полный documentation gate ещё не закрыты.

**Gate:** вертикальный Dojo-сценарий работает на реальном устройстве, а ключевые performance и security assumptions подтверждены измерениями.

## Правило старта production

Production начинается только после прохождения Gate 1–3, стабилизации C ABI, утверждения offline-протокола и повторной оценки MVP. Непройденный gate приводит к явному изменению платформы, скоупа или требований, а не к молчаливому переносу риска в разработку.
