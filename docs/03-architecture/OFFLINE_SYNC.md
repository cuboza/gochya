# Offline Command and Reconciliation Protocol

> Сервер остаётся авторитетом. Клиент хранит кэш состояния и журнал намерений, но никогда не загружает на сервер готовое состояние питомца, кошелька или инвентаря.

## 1. Модель

Каждый server aggregate (`player`, `pet`, `inventory`) имеет монотонную `revision`. Офлайн-действие записывается как команда:

```json
{
  "operationId": "uuid",
  "aggregateType": "pet",
  "aggregateId": "uuid",
  "baseRevision": 42,
  "operationType": "feed",
  "arguments": { "itemId": "apple" },
  "clientWallTime": "2026-07-22T12:00:00Z",
  "clientMonotonicOffsetMs": 120340,
  "schemaVersion": 1
}
```

`operationId` является idempotency key. `clientWallTime` используется для UX и anomaly detection, но не определяет награду или порядок серверных транзакций.

## 2. Разрешённые offline-действия MVP

| Действие | Offline | Условие reconcile |
|---|---|---|
| Feed/clean/play | да | предмет и revision подтверждаются сервером |
| Start sleep | да | сервер назначает восьмичасовое окно |
| Strength training | протоколом предусмотрено, endpoint V1 ещё не принимает | сервер пересчитывает награду, действует дневной cap |
| Просмотр/экипировка локального UI | частично | сервер подтверждает ownership |
| PvP, breeding, gacha/IAP | нет | требуется онлайн до начала операции |
| Выдача валюты/предмета | нет | только серверная транзакция |

## 3. Reconcile

1. Клиент отправляет команды в порядке локального sequence number вместе с последней известной revision.
2. Сервер дедуплицирует по `(player_id, operation_id)`.
3. Сервер блокирует aggregate или использует optimistic compare-and-swap по revision.
4. Decay пересчитывается по server time через Shared Core.
5. Каждая команда валидируется против актуального состояния и исполняется через Shared Core.
6. Расход предмета и изменение состояния фиксируются в одной DB transaction.
7. Ответ содержит статус каждой команды, новую revision и canonical snapshot.
8. Клиент заменяет кэш canonical snapshot и удаляет только подтверждённые команды.

Статусы: `APPLIED`, `ALREADY_APPLIED`, `REJECTED_PRECONDITION`, `REJECTED_EXPIRED`, `REJECTED_INVALID`, `RETRYABLE`.

## 4. Конфликты нескольких устройств

- Серверный порядок commit является окончательным.
- Команда со старой revision не применяется автоматически, если её preconditions могли измениться.
- Care V1 не перебазирует команду автоматически: устаревшая `baseRevision`
  получает `REJECTED_PRECONDITION`.
- Расход одного и того же предмета некоммутативен, поэтому один из конкурентных
  запросов с одной revision применяется, а второй отклоняется.
- Клиент показывает результат reconcile, если визуально обещанный offline-эффект был изменён или отклонён.
- Device wall-clock rollback/forward не увеличивает награды и не сокращает cooldown.

## 5. Ограничения и хранение

- Максимум 100 pending-команд или 24 часа offline progression — что наступит раньше.
- Команды хранятся в encrypted platform storage/SQLite; auth secrets не входят в payload.
- Сервер хранит idempotency result не меньше максимального offline window плюс 7 дней.
- Удаление аккаунта инвалидирует очередь; команды другого account ID никогда не мигрируют автоматически.

## 6. API

```text
POST /v1/sync/commands
If-Match: <baseRevision первой команды>

{ deviceId, commands[] }
→ { results[], canonicalSnapshots[], newRevision, serverTime }
```

Реализованный care V1 принимает 1–100 команд и не больше 256 KiB, требует
канонический непустой `deviceId` до 128 bytes и разрешает одному batch
обращаться только к одному питомцу. Команды применяются по порядку, поэтому
`baseRevision` следующей команды должен учитывать предыдущую `APPLIED`.
Поддерживаются:

- `feed` с `apple | steak | energy_drink`;
- `clean` без предмета или с `soap | shampoo`;
- `play` и `sleep` без предмета.

Валидное окно `clientWallTime` — последние 24 часа с допуском пяти минут в
будущее. Оно служит только admission check: decay и восьмичасовой Sleep считаются
от целосекундного server time. Не-Sleep care завершает текущий Sleep.

Повтор идентичного `operationId + payload` безопасен: применённая команда
возвращается как `ALREADY_APPLIED`, ранее отклонённая сохраняет свой исходный
статус. Повтор `operationId` с другим payload возвращает
`REJECTED_INVALID/idempotency_conflict`. Ответ всегда содержит финальный
canonical snapshot питомца, даже если сохранённый результат retry относится к
более ранней revision.

## 7. Definition of Done

- Повтор одного batch не удваивает эффект и расход.
- Два устройства не могут потратить один предмет.
- Смена timezone и wall clock не меняет начисление.
- Crash клиента после server commit восстанавливается повторной отправкой.
- Unit и PostgreSQL integration tests покрывают duplicate, partial rejection,
  конфликт payload, устаревшую revision и concurrent devices.
