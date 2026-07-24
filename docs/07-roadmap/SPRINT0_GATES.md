# Sprint 0 Gate Checklists

> Gate закрывается только приложенными артефактами и измерениями. Формулировка «работает у разработчика» без build ID, устройства и результата теста не считается.

## Общий формат evidence

Для каждого запуска сохранить:

- дату, commit SHA и CI/build ID;
- модель устройства, OS и firmware;
- build type и версии toolchain/SDK;
- команды воспроизведения;
- сырые логи измерений и краткий вывод;
- принятое решение: `PASS`, `FAIL`, `PASS_WITH_DEVIATION`;
- owner и срок устранения deviation.

## Gate 1 — Wear OS runtime и performance

Целевые устройства: Galaxy Watch 6 и 7; Watch4 используется как нижняя compatibility check.

- [ ] APK устанавливается и запускается без developer-only runtime flags.
- [ ] Filament показывает одну production-representative glTF-модель и обязательные VFX.
- [ ] Одновременно читаются accelerometer, gyroscope и доступный HR provider.
- [ ] Cold start p50/p95 измерен на 20 запусках; p95 ≤ 2 секунды либо budget официально пересмотрен.
- [ ] Active FPS p50/p5 измерен; нет thermal degradation за 10 минут.
- [ ] Dojo 16 секунд потребляет ≤ 0.5% батареи по воспроизводимой методике.
- [ ] Background drain и ambient lifecycle измерены отдельно.
- [ ] Memory peak ≤ 150 MB, APK ≤ 20 MB либо budgets пересмотрены до production.

**Fail action:** упростить renderer/model/VFX или изменить device floor; молчаливое снижение качества запрещено.

## Gate 2 — watchOS feasibility

- [ ] Rust toolchain воспроизводимо собирает `arm64_32-apple-watchos`.
- [ ] Сгенерированный header соответствует `CORE_ABI.md`.
- [ ] Swift вызывает ABI smoke function на реальном Apple Watch S9.
- [ ] Golden fixture совпадает с native/server результатом byte-for-byte.
- [ ] SpriteKit/Rive prototype отображает production-representative персонажа.
- [ ] Зафиксированы cold start, FPS, memory и 16-second battery measurement.
- [ ] Проверено поведение при конфликте `HKWorkoutSession`.

**Fail action:** до Beta выбрать поддерживаемую альтернативу core integration или снять watchOS с roadmap.

## Gate 3 — Samsung HR и distribution approval

- [ ] Partner request отправлен; сохранены request ID, package name и certificate fingerprint.
- [ ] Подтверждено поведение production build без developer mode.
- [ ] На Watch4/6/7 измерены time-to-first-sample, sample frequency, gaps и contact failures.
- [ ] Для 30 валидных сессий рассчитан `HR_present`; p95 сессий проходит заданный порог.
- [ ] Одновременная работа HR + motion не нарушает performance Gate 1.
- [ ] Play Integrity verdicts проверены на целевых часах и документирована server policy.
- [ ] Выбран один fallback при отсутствии approval: смена первой платформы, отключение Dojo на Wear OS либо пересмотр heart gate.

**Fail action:** активировать заранее выбранный fallback и пересчитать MVP; Gate нельзя закрыть обещанием будущего approval.

## Gate 4 — классификатор и датасет

- [ ] Есть consent form, retention policy и процедура удаления записей добровольца.
- [ ] Определены протокол записи, устройство, sampling rates и labels.
- [ ] Собраны минимум 5 участников и сбалансированный pilot по трём MVP-классам.
- [ ] Train/validation/test разделены по людям, а не по отдельным движениям одного человека.
- [ ] Зафиксированы accuracy, macro-F1, confusion matrix и false-accept rate.
- [ ] Golden fixtures экспортируются в privacy-safe feature summary schema.
- [ ] Thresholds приняты до масштабирования датасета.

**Fail action:** уменьшить число классов или изменить UX повторной записи; не маскировать низкое качество редкостью карты.

## Gate 5 — end-to-end vertical slice

```text
Wear OS → nonce/attestation → Dojo payload → Go API
→ Rust Core → PostgreSQL/ledger → Flutter inventory
```

- [ ] Повтор payload не создаёт вторую карту.
- [ ] Изменённый payload с прежней подписью отклоняется.
- [ ] Истёкший nonce отклоняется.
- [ ] Потеря ответа после commit восстанавливается idempotent retry.
- [ ] Schema/ABI mismatch возвращает управляемую ошибку.
- [ ] Raw sensor time series отсутствует в network capture, logs и crash reports.
- [x] Trace ID связывает preflight, submit, core call, DB transaction и client response.

## Решение о старте production

Подписи обязательны от Tech Lead, Backend, Wear OS, Security/Privacy и Product. Gate 1–3 и 5 должны быть `PASS`; Gate 4 может быть `PASS_WITH_DEVIATION` только с уменьшенным числом классов и утверждённым UX fallback.
