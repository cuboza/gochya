# Shared Core C ABI

> Нормативный контракт границы Rust ↔ Go/Swift/Kotlin/Dart. `CORE_SPEC.md` описывает доменную модель; этот файл определяет только стабильный внешний ABI.

## 1. Версионирование

- ABI использует Semantic Versioning отдельно от версии игры.
- `gochya_abi_version()` возвращает packed `major.minor.patch` как `u32`.
- Несовпадение `major` запрещает загрузку библиотеки.
- Добавление функции или поля через новую структуру повышает `minor`; изменение размера, порядка или смысла существующего поля повышает `major`.
- Каждый payload содержит `schema_version`; ABI version и data schema version не смешиваются.

Текущая реализация: ABI `2.4.0` (`0x00020400`), schema `1`. Сгенерированный
artifact — `core/ffi/gochya_core.h`; `core/build.rs` сравнивает его с результатом
`cbindgen` при каждой сборке.

## 2. Разрешённые типы

Через ABI пересекают границу только:

- fixed-width integers `uint8_t`…`uint64_t`, `int32_t`, `int64_t`;
- `float` при явно определённых правилах округления;
- `uint8_t` для boolean (`0`/`1`);
- `#[repr(C)]` POD-структуры без указателей на Rust-owned memory;
- opaque handles как `uint64_t`;
- caller-owned buffers `(ptr, capacity, out_len)`.

Запрещены Rust references, `String`, `Vec`, slices, trait objects, `Option`, `Result`, Rust enum без `repr`, `usize/isize` и возврат крупных структур by value.

## 3. Layout

- Все ABI-структуры имеют `#[repr(C)]` и статический size/alignment assertion для каждого target.
- UUID передаётся как `uint8_t[16]`.
- Время — Unix milliseconds в `int64_t`; monotonic time никогда не сериализуется как wall clock.
- Enum передаётся как явно документированный `uint8_t`/`uint16_t`; числовые значения не переиспользуются.
- Reserved bytes обнуляются вызывающим и позволяют расширить структуру без изменения размера.

Пример:

```c
typedef struct {
  uint32_t struct_size;
  uint16_t schema_version;
  uint8_t technique_type;
  uint8_t reserved0;
  float peak_accel_mps2;
  float precision;
  uint8_t reserved[16];
} GochyaPunchMetricsV1;
```

`struct_size` проверяется каждой функцией до чтения остальных полей.

## 4. Ownership и буферы

- По умолчанию память выделяет и освобождает caller.
- Core не сохраняет входные указатели после возврата функции.
- Для variable-size результата caller сначала вызывает `*_required_size`, затем передаёт буфер.
- Недостаточный буфер возвращает `GOCHYA_BUFFER_TOO_SMALL` и требуемый размер в `out_len`; частичный результат запрещён.
- Если Rust когда-либо возвращает owned handle, для него обязательна парная `gochya_*_free(handle)`; утечка handle проверяется тестами.

## 5. Errors и panic safety

```c
typedef int32_t GochyaStatus;

enum {
  GOCHYA_OK = 0,
  GOCHYA_INVALID_ARGUMENT = 1,
  GOCHYA_BUFFER_TOO_SMALL = 2,
  GOCHYA_SCHEMA_MISMATCH = 3,
  GOCHYA_DOMAIN_REJECTED = 4,
  GOCHYA_INTERNAL_ERROR = 255
};
```

- Все экспортированные функции возвращают `GochyaStatus`; доменный результат пишется в out-параметр.
- Null, size, enum range и finite-float checks выполняются до dereference.
- Каждая `extern "C"` функция обёрнута в `catch_unwind`; unwind через ABI запрещён.
- Текст ошибки доступен через caller-owned diagnostic buffer и не используется для программной логики.

## 6. Threading и состояние

- Чистые формулы thread-safe и не используют global mutable state.
- Mutable opaque handle принадлежит одному потоку, если функция явно не документирована как thread-safe.
- RNG создаётся как opaque handle или передаётся явным seed; Rust `Rng` никогда не возвращается by value.
- Инициализация идемпотентна. Host logging callback регистрируется до конкурентных вызовов.

## 7. Детерминизм

- Запрещены platform RNG, locale-dependent parsing и незафиксированная iteration order.
- Float operations имеют зафиксированную последовательность; запрещены platform-specific fast-math оптимизации для golden paths.
- Golden fixtures содержат input bytes, seed, expected output bytes и schema version.

## 8. Генерация и проверка

- `cbindgen` генерирует `gochya_core.h`; ручное редактирование header запрещено.
- CI сравнивает сгенерированный header с committed artifact.
- Для каждого target проверяются `sizeof`, `alignof`, enum values и один golden fixture.
- Обязательные consumers: Go/cgo, Dart FFI, Kotlin/JNI и Swift spike.
- Любое изменение ABI сопровождается changelog и migration note.

## 9. Definition of Done

- Header генерируется воспроизводимо.
- Ни один exported symbol не содержит Rust-specific ABI types.
- Address/undefined behavior sanitizers проходят native ABI harness.
- Golden fixture имеет одинаковые bytes на server, iOS phone и Android/Wear OS; watchOS fixture обязателен в Gate 2.

## 10. Реализованный surface

Стабилизированный ABI-срез экспортирует:

- `gochya_abi_version`;
- `gochya_validate_heart_v1`;
- `gochya_quality_score_v1`;
- `gochya_compute_vitality_v1`;
- `gochya_compute_goals_v1`;
- `gochya_compute_activity_v1`;
- `gochya_derive_technique_v1`;
- `gochya_generate_loot_technique_v1`;
- `gochya_simulate_combat_v1`;
- `gochya_breed_v1`;
- `gochya_generate_starter_genome_v1`;
- `gochya_advance_needs_v1`;
- `gochya_apply_care_v1`.
- `gochya_apply_rest_v1`.

Операции со структурными envelope используют `struct_size`/`schema_version`;
все функции проверяют применимые null, schema, enum range и finite-float
условия, возвращают `GochyaStatus`, записывают результат через caller-owned
out-параметр и защищены `catch_unwind`. Starter-функция принимает только
скалярные `element`/`seed` и заполняет уже версионированный на уровне symbol
`GochyaGenomeV1`, поэтому consumer дополнительно проверяет ABI version. Нативный
C harness находится в `core/tests/abi_smoke.c`, а серверный consumer — в
`server/internal/corebridge`. `gochya_derive_technique_v1` атомарно возвращает
тип, редкость, урон, скорость, stamina cost, crit chance и quality, чтобы сервер
не дублировал формулы карты.

`gochya_generate_loot_technique_v1(seed, max_rarity, out_stats)` возвращает тот
же `GochyaTechniqueStatsV1` для server-authoritative игровой добычи.
`max_rarity` принимает Common…Epic; Legendary/Mythic отклоняются.

## 11. Activity V1 wire schema

`gochya_compute_activity_v1(activity, goals, streak_days, out_result)` атомарно
вычисляет полный итог синхронизации активности: дневную vitality и gains для
`STR`, `AGI`, `END`, `FOC`.

`gochya_compute_goals_v1(baseline, out_goals)` рассчитывает адаптивные goals из
14-дневного baseline той же формулой Core, не оставляя её копию в consumer.

- `GochyaWorkoutV1` содержит стабильный числовой kind, длительность в минутах и
  calories;
- `GochyaActivityInputV1` содержит полный `DailyActivitySnapshot`, ровно восемь
  workout slots и элемент питомца, необходимый для resonance;
- `GochyaDailyGoalsV1` переиспользуется без изменения;
- `GochyaActivityResultV1` содержит vitality и четыре signed stat gains.

Фиксированные размеры V1:

| Структура | `sizeof` |
|---|---:|
| `GochyaWorkoutV1` | 8 |
| `GochyaPersonalBaselineV1` | 32 |
| `GochyaActivityInputV1` | 120 |
| `GochyaActivityResultV1` | 32 |

FFI отклоняет больше восьми тренировок, неизвестные source/element, sleep quality
или stress level больше 100 и non-finite `sleep_hours`. Неизвестный workout kind
допустим: его длительность входит в общий объём, но не получает type-specific
gain или resonance. В gains участвуют только первые три workout slots согласно
дневному cap доменной формулы.

### Миграция 1.1.0 → 1.2.0

Изменение добавочное: существующие структуры и
`gochya_compute_vitality_v1` не изменены. Consumers 1.1 могут продолжать
использовать старый symbol; consumers, которым нужны stat gains, проверяют
`gochya_abi_version() >= 0x00010200` и переходят на
`gochya_compute_activity_v1`.

### Миграция 1.2.0 → 1.3.0

Добавлены `GochyaPersonalBaselineV1` и `gochya_compute_goals_v1`; существующие
activity payloads и функции не изменены. Серверу с adaptive goals требуется
`gochya_abi_version() >= 0x00010300`.

### Миграция 1.3.0 → 2.0.0

Добавлен `gochya_generate_loot_technique_v1`; существующие структуры и symbols
не изменены. Одновременно `base_damage` Dojo-карты приведён к combat-шкале
множителем 100 — это исправление значения результата существующей функции,
поэтому ABI major повышен согласно правилам §1, а consumers должны обновить
проверку версии и golden fixtures.

## 12. Combat V1 wire schema

`gochya_simulate_combat_v1(match, seed, out_result)` принимает компактный
боевой snapshot, а не persistent-типы целиком:

- `GochyaCombatCardV1` содержит только используемые формулой card stats и effect;
- `GochyaCombatLoadoutV1` содержит pet stats, gear bonuses, pet element, affinity, mood,
  signature index и ровно 5 карт;
- `GochyaCombatMatchV1` содержит два loadout и mode;
- `GochyaCombatResultV1` содержит winner, final HP, seed и фиксированный массив
  из 20 `GochyaCombatRoundV1`.

Идентификаторы игрока/питомца/карт, timestamps, rarity и economy metadata через
combat ABI не передаются: сервер хранит их в match snapshot, но Rust-формуле они
не нужны. Это исключает случайную зависимость результата от небоевых полей.

Фиксированные размеры V1:

| Структура | `sizeof` |
|---|---:|
| `GochyaCombatCardV1` | 20 |
| `GochyaCombatLoadoutV1` | 144 |
| `GochyaCombatMatchV1` | 312 |
| `GochyaCombatRoundV1` | 12 |
| `GochyaCombatResultV1` | 280 |

FFI отклоняет mood > 100, signature index > 4, неизвестные mode/element/
technique/effect enum, non-finite или отрицательные card floats и crit chance
вне `[0, 0.35]`. Один `MatchV1 + seed` даёт byte-for-byte стабильный replay;
Rust unit test, C harness и Go/cgo consumer используют один golden vector.

## 13. Breeding V1 wire schema

`gochya_breed_v1(input, seed, out_result)` принимает два полных genome snapshot,
флаги mutation/hybrid catalyst и вычисленный сервером `inbreeding_coeff`.
Результат содержит offspring genome, 4–24 часа инкубации и 14-bit mutation
mask. IDs родителей, wallet, предметы, cooldown и lineage через ABI не
передаются: они проверяются и фиксируются PostgreSQL-транзакцией.

Фиксированные размеры V1:

| Структура | `sizeof` |
|---|---:|
| `GochyaVisualGenesV1` | 16 |
| `GochyaStatPotentialsV1` | 8 |
| `GochyaGenomeV1` | 40 |
| `GochyaBreedInputV1` | 112 |
| `GochyaBreedResultV1` | 64 |

FFI отклоняет неизвестные enum, потенциалы/насыщенность больше 100, hue больше
360, boolean не из `0/1` и коэффициент родства больше 3.

### Миграция 2.0.0 → 2.1.0

Изменение добавочное: экспортированы новые breeding POD-структуры и
`gochya_breed_v1`; существующие структуры и symbols не изменены. Consumers,
которым нужен breeding, проверяют `gochya_abi_version() >= 0x00020100`.

## 14. Starter genome V1

`gochya_generate_starter_genome_v1(element, seed, out_genome)` создаёт
авторитетный геном первого питомца. В текущем content release допустимы только
числовые ID `0=Fire`, `1=Water`, `2=Earth`; остальные значения отклоняются.
Выбранный элемент задаёт стабильный визуальный preset, а server-generated seed
детерминированно варьирует разрешённые косметические индексы и начальную
technique affinity.

Стартовый геном всегда Common, без ability, generation `0` и с потенциалами
`60/60/60/60`. Функция возвращает существующий `GochyaGenomeV1`, не принимает
идентификаторы игрока/яйца и не определяет время инкубации: владение,
одноразовость выбора и пятисекундный tutorial timer фиксирует серверная
PostgreSQL-транзакция.

### Миграция 2.1.0 → 2.2.0

Изменение добавочное: экспортирован
`gochya_generate_starter_genome_v1`; существующие структуры и symbols не
изменены. Consumers, которым нужна выдача стартового питомца, проверяют
`gochya_abi_version() >= 0x00020200`.

## 15. Needs and care V1

`GochyaNeedsStateV1` — 56-byte POD snapshot с четырьмя потребностями, четырьмя
fixed-point остатками decay, непрерывным `zero_streak_seconds` и boolean-флагами
sleeping/Weakness. Reserved bytes должны быть нулевыми.

`gochya_advance_needs_v1(input, elapsed_seconds, out_state)` применяет
детерминированный decay и Weakness. Один вызов принимает не больше 86 400
секунд; consumer обязан разбить более длинный интервал на chunks, передавая
полный возвращённый state дальше.

`gochya_apply_care_v1(input, action, item, out_state)` применяет release-scoped
таблицу эффектов. Числовые ID действий: `0=Feed`, `1=Clean`, `2=Play`,
`3=Sleep`; предметов: `0=None`, `1=Apple`, `2=Steak`, `3=EnergyDrink`,
`4=Soap`, `5=Shampoo`. Неизвестные ID возвращают `GOCHYA_INVALID_ARGUMENT`, а
несовместимые пары — `GOCHYA_DOMAIN_REJECTED`. Инвентарь, время и revision
остаются за пределами ABI.

`gochya_apply_rest_v1(input, sleep_minutes, sleep_quality, out_state)`
применяет ночь сна владельца к потребностям питомца по `CORE_FORMULAS.md` §1.8.
Качество — шкала `0..100`; значение выше возвращает `GOCHYA_INVALID_ARGUMENT`.
Восстанавливается только Energy, короткая или плохая ночь снимает Mood.
Функция чистая: идемпотентность «одна ночь — один раз» обеспечивает consumer.

### Миграция 2.3.0 → 2.4.0

Изменение добавочное: экспортирован `gochya_apply_rest_v1`; структуры и
существующие symbols не изменены, поэтому старый consumer продолжает работать
без правок. Тем, кому нужен rest, проверять `gochya_abi_version() >= 0x00020400`.

### Миграция 2.2.0 → 2.3.0

Изменение добавочное: экспортированы `GochyaNeedsStateV1`,
`gochya_advance_needs_v1` и `gochya_apply_care_v1`; существующие структуры и
symbols не изменены. Consumers, которым нужны decay/care, проверяют
`gochya_abi_version() >= 0x00020300`.
