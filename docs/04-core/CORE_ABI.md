# Shared Core C ABI

> Нормативный контракт границы Rust ↔ Go/Swift/Kotlin/Dart. `CORE_SPEC.md` описывает доменную модель; этот файл определяет только стабильный внешний ABI.

## 1. Версионирование

- ABI использует Semantic Versioning отдельно от версии игры.
- `gochya_abi_version()` возвращает packed `major.minor.patch` как `u32`.
- Несовпадение `major` запрещает загрузку библиотеки.
- Добавление функции или поля через новую структуру повышает `minor`; изменение размера, порядка или смысла существующего поля повышает `major`.
- Каждый payload содержит `schema_version`; ABI version и data schema version не смешиваются.

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
