# GOCHYA Shared Core

Детерминированная игровая логика для телефона, часов и Go-сервера.

Реализованный Sprint 0 срез:

- доменные типы питомца, генома, Technique Card, активности и боя;
- PCG-XSH-RR 64/32 с фиксированным stream;
- heart gate, quality score, rarity, adaptive goals, vitality и activity stat gains;
- стабильные WorkoutKind ID и resonance-таблица тренировок/стихий;
- MVP-таблица стихий и детерминированный casual combat;
- versioned C ABI для heart/quality/activity/combat;
- unit, property, invariant, golden и C ABI smoke tests.

Проверка из корня репозитория:

```bash
bash tools/check-core.sh
```

Header генерируется `cbindgen` при сборке и сверяется с
`core/ffi/gochya_core.h`. После намеренного ABI-изменения обновить artifact:

```bash
GOCHYA_UPDATE_HEADER=1 cargo build -p gochya-core
```

Аппаратные Gate 1–3 из `docs/07-roadmap/SPRINT0_GATES.md` эта сборка не закрывает.
