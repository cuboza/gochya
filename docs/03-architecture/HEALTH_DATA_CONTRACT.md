# Health Data Source and Deduplication Contract

> Нормативный контракт нормализации HealthKit, Health Connect, Health Services и Samsung Health перед расчётом Vitality.

## 1. Source of truth по платформам

| Метрика | iOS phone | Apple Watch | Android phone | Galaxy Watch realtime |
|---|---|---|---|---|
| Шаги/сон/тренировки | HealthKit | HealthKit как source record | Health Connect | Health Services только для текущей сессии |
| Active calories | HealthKit | HealthKit | Health Connect | Health Services session estimate |
| HR для Dojo | не используется | live workout API | не используется | Samsung Health Sensor SDK |

Телефон отправляет серверу нормализованный дневной snapshot. Часы могут отправлять
Dojo evidence напрямую, но не должны параллельно начислять дневную Vitality за те же
source records.

## 2. Нормализованная запись

```text
source_platform
source_app_id
source_device_id_hash
source_record_id_hash
metric_type
start_time_utc
end_time_utc
value
unit
last_modified_at
snapshot_schema_version
```

Хэш source ID используется для дедупликации и не является доказательством подлинности.
Сервер не хранит имя тренировки, маршрут, координаты или сырой HR time series.

## 3. Дедупликация

1. Точное совпадение `source_platform + source_record_id_hash` считается одной записью; побеждает более новый `last_modified_at`.
2. При отсутствии стабильного source ID записи одного типа с overlap ≥ 80%, одинаковым source device и отклонением value ≤ 5% считаются дублем.
3. Данные, синхронизированные Samsung Health в Health Connect, не складываются с отдельным Samsung snapshot: используется Health Connect record provenance.
4. Watch-derived record имеет приоритет только при доказанном overlap; правило «часы всегда побеждают весь день» запрещено.
5. Удаление/исправление source record приводит к пересчёту дневного total, но уже потраченная Vitality не создаёт отрицательный wallet автоматически — формируется anomaly/reconciliation event.

## 4. Идемпотентное начисление

- Сервер хранит canonical daily total и `vitality_awarded`.
- Новое начисление: `max(0, computed_total - vitality_awarded)`.
- Snapshot имеет deterministic fingerprint после сортировки нормализованных записей.
- Повтор fingerprint — no-op.
- День определяется в сохранённой пользовательской timezone, но границы переводятся сервером в UTC; смена timezone действует со следующего server day.

## 5. Privacy и retention

- Сервер принимает только минимальные агрегаты и provenance hashes.
- Source record hashes удаляются вместе с дневным snapshot по retention policy.
- Отзыв consent прекращает новые чтения; уже серверные данные удаляются или сохраняются только согласно явно описанной legal basis.
- Данные health не используются для рекламы, device fingerprinting или продажи сегментов.

## 6. Ошибки и degraded mode

- Нет permission/API → базовый progression без health bonus.
- Source временно недоступен → последнее значение не растягивается на новый день.
- Частичный день помечается `incomplete`; повторная синхронизация обновляет canonical total.
- Невозможная unit/schema версия отклоняется целиком, без частичного начисления.

## 7. Definition of Done

- Один день, синхронизированный watch → phone → Health Connect/HealthKit, начисляется один раз.
- Обновлённая тренировка заменяет предыдущую версию.
- Смена timezone не создаёт второй дневной cap.
- Повтор/перестановка source records даёт одинаковый fingerprint и Vitality.
- Property tests покрывают overlap, correction, deletion, duplicate device и partial day.
