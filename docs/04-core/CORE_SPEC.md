# SHARED CORE — Спецификация

> Контракт общего ядра. Все типы, public API, инварианты. Ядро реализуется на Rust (рекомендуется) или C#/.NET NativeAOT. **Это — единственный источник истины для игровой логики.**

---

## 1. ПРИНЦИПЫ

1. **Детерминизм:** одинаковые входы → одинаковые выходы на всех платформах. Случайность — только через `Rng` с явным seed.
2. **Headless:** ядро ничего не знает про UI/сенсоры/сеть. Только чистая логика.
3. **Тестируемость:** 100% public API покрыты unit + property тестами.
4. **Сериализуемость:** все persistent-типы сериализуются в JSON/MessagePack с версионированием (`schemaVersion`).
5. **C-ABI:** публичный API экспортируется через C-интерфейс (`#[repr(C)]`, `extern "C"`) для FFI во все клиенты.
6. **Без паник в FFI:** все public `extern "C"` функции возвращают код ошибки, не паникуют.

---

## 2. ЯЗЫК И СБОРКА

**Рекомендация: Rust** (статические гарантии, отличный FFI, малый runtime, легко кросс-компилировать).

| Таргет | Выход |
|---|---|
| `aarch64-apple-ios` (watchOS/iOS) | `libgochya_core.a` → `.xcframework` |
| `aarch64-linux-android` (Wear OS) | `libgochya_core.so` → Unity plugin |
| `x86_64-unknown-linux-gnu` (server) | `.a` или cgo |
| `wasm32-unknown-unknown` (опц. сервер) | `.wasm` |
| `x86_64-*` (CI/тесты) | native binary |

Альтернатива — C#/.NET 8 NativeAOT (если команда сильнее в C#).

---

## 3. ОСНОВНЫЕ ТИПЫ

### 3.1. Базовые enum'ы
```rust
#[repr(C)]
pub enum Element: u8 {
    Fire, Water, Earth, Air,
    Light, Dark, Arcane,
    // Гибриды:
    Steam, Magma, Storm, ...
}

#[repr(C)]
pub enum Rarity: u8 {
    Common, Uncommon, Rare, Epic, Legendary, Mythic,
}

#[repr(C)]
pub enum TechniqueType: u8 {
    Jab, Hook, Uppercut, Cross, Kick, Elbow, Block,
}

#[repr(C)]
pub enum Stat: u8 { Str, Agi, End, Foc }

#[repr(C)]
pub enum Effect: u8 {
    None, Stun(u8), Bleed(u8), Crit(f32), Slow(u8), Heal(u16),
}
```

### 3.2. Геном (для бридинга)
```rust
#[repr(C)]
pub struct Genome {
    pub visual:        VisualGenes,
    pub stats:         StatPotentials,    // скрытые потенциалы STR/AGI/END/FOC
    pub element:       Element,
    pub tech_affinity: TechniqueType,
    pub rarity:        Rarity,
    pub ability:       Option<Ability>,
    pub generation:    u32,
}

#[repr(C)]
pub struct VisualGenes {
    pub body_shape:  u8,
    pub palette_hue: u16,        // 0..360
    pub palette_sat: u8,         // 0..100
    pub pattern:     u8,
    pub size:        u8,
    pub eye_style:   u8,
    pub aura:        u8,
}

#[repr(C)]
pub struct StatPotentials {
    pub str_pot: u8,   // 0..100 (потолок для выращивания)
    pub agi_pot: u8,
    pub end_pot: u8,
    pub foc_pot: u8,
}
```

### 3.3. Питомец
```rust
#[repr(C)]
pub struct Pet {
    pub id:           [u8; 16],   // UUID
    pub genome:       Genome,
    pub name:         [u8; 32],   // fixed-size для C-ABI
    pub stage:        Stage,
    pub level:        u32,
    pub xp:           u64,
    pub needs:        Needs,
    pub stats:        Stats,       // текущие (выращенные)
    pub created_at:   u64,         // unix ts
    pub last_sync:    u64,
}

#[repr(C)]
pub enum Stage: u8 { Egg, Baby, Teen, Adult, Premium }

#[repr(C)]
pub struct Needs {
    pub hunger:  u8,   // 0..100
    pub energy:  u8,
    pub hygiene: u8,
    pub mood:    u8,
}

#[repr(C)]
pub struct Stats {
    pub str: u32,
    pub agi: u32,
    pub end: u32,
    pub foc: u32,
}
```

### 3.4. Technique Card (результат записи удара)
```rust
#[repr(C)]
pub struct PunchMetrics {
    pub peak_accel:     f32,   // м/с²
    pub exec_time:      f32,   // секунды до пика
    pub precision:      f32,   // 0..1 (соответствие шаблону)
    pub combo_len:      u8,    // число различных ударов
    pub rhythm_score:   f32,   // 0..1
    pub technique_type: TechniqueType,
}

#[repr(C)]
pub struct HeartRateEvidence {
    pub baseline:        u16,   // bpm до записи
    pub mean:            u16,   // bpm за окно
    pub present:         f32,   // 0..1, доля валидных сэмплов
    pub confidence:      f32,   // 0..1, контакт с кожей
    pub delta:           i16,   // mean − baseline
}

#[repr(C)]
pub struct TechniqueCard {
    pub id:           [u8; 16],
    pub type_:        TechniqueType,
    pub element:      Element,
    pub rarity:       Rarity,
    pub base_damage:  f32,
    pub speed:        f32,
    pub stamina_cost: u16,
    pub effect:       Effect,
    pub quality:      u8,      // 0..100
    pub owner_id:     [u8; 16],
    pub created_at:   u64,
}
```

### 3.5. Активность (для Симбиоза)
```rust
#[repr(C)]
pub struct DailyActivitySnapshot {
    pub steps:           u32,
    pub sleep_minutes:   u16,
    pub active_calories: u16,
    pub workouts:        WorkoutSummary,      // массив фиксированной длины через len+ptr
    pub workout_count:   u8,
    pub avg_hr:          u16,
    pub stress_level:    u8,                   // 0..100
    pub floors:          u16,
    pub stand_hours:     u8,
    pub source:          DataSource,
    pub timestamp:       u64,
}

#[repr(C)]
pub enum DataSource: u8 { Watch, Phone }

#[repr(C)]
pub struct WorkoutSummary {
    pub kind:      u8,    // running, cycling, strength, yoga, ...
    pub duration_min: u16,
    pub calories:    u16,
}

#[repr(C)]
pub struct PersonalBaseline {
    pub steps_14d_ma:  u32,
    pub sleep_14d_ma:  f32,
    pub cals_14d_ma:   u16,
}

#[repr(C)]
pub struct DailyGoals {
    pub steps: u32,
    pub sleep_hours: f32,
    pub cals: u16,
}

#[repr(C)]
pub struct StatGains {
    pub str: u16,
    pub agi: u16,
    pub end: u16,
    pub foc: u16,
}
```

### 3.6. Бой (авто-баттлер)
```rust
#[repr(C)]
pub struct Loadout {
    pub pet_id:       [u8; 16],
    pub pet_stats:    Stats,
    pub pet_genome:   Genome,
    pub cards:        [TechniqueCard; 5],  // 4 обычных + 1 signature
    pub gear:         GearSummary,
}

#[repr(C)]
pub struct GearSummary {
    pub str_bonus:  i16,
    pub agi_bonus:  i16,
    pub end_bonus:  i16,
    pub foc_bonus:  i16,
    pub element:    Element,
}

#[repr(C)]
pub struct Match {
    pub loadout_a: Loadout,
    pub loadout_b: Loadout,
    pub mode:      MatchMode,
}

#[repr(C)]
pub enum MatchMode: u8 { Casual, Ranked, Tournament }

#[repr(C)]
pub struct MatchResult {
    pub winner:    Winner,
    pub rounds:    RoundLog,         // массив фиксированной длины
    pub round_count: u8,
    pub final_hp_a: u16,
    pub final_hp_b: u16,
    pub seed:      u64,
}

#[repr(C)]
pub enum Winner: u8 { A, B, Draw }

#[repr(C)]
pub struct RoundLog {
    pub card_a_idx: u8,
    pub card_b_idx: u8,
    pub damage_a_to_b: u16,
    pub damage_b_to_a: u16,
    pub effect_triggered: Effect,
}
```

---

## 4. PUBLIC API (C-ABI)

Сигнатуры приведены в Rust-формате; в FFI превращаются в `extern "C"`. **Все функции принимают/возвращают C-совместимые типы.**

### 4.1. RNG (детерминированный)
```rust
pub fn rng_new(seed: u64) -> Rng;
pub fn rng_next(r: &mut Rng) -> u64;
pub fn rng_range(r: &mut Rng, lo: u32, hi: u32) -> u32;
```

### 4.2. Потребности и эволюция
```rust
pub fn tick_needs(needs: &mut Needs, dt_seconds: u64, is_sleeping: bool);
pub fn mood_multiplier(mood: u8) -> f32;
pub fn xp_to_next(level: u32) -> u64;
pub fn can_evolve(pet: &Pet) -> EvolutionCheck;
pub fn evolve(pet: &mut Pet, branch_hint: Option<Branch>) -> Result<(), CoreError>;
pub fn apply_care_action(pet: &mut Pet, action: CareAction) -> Result<(), CoreError>;
```

### 4.3. Dojo / Technique Card
```rust
pub fn validate_heart(e: &HeartRateEvidence) -> HeartVerdict;
pub fn spirit_bonus(e: &HeartRateEvidence) -> f32;
pub fn quality_score(m: &PunchMetrics, e: &HeartRateEvidence) -> u8; // 0..100
pub fn rarity_from_quality(q: u8) -> Rarity;
pub fn create_technique_card(
    m: &PunchMetrics, e: &HeartRateEvidence, owner: &[u8;16], ts: u64, rng: &mut Rng,
) -> TechniqueCard;
```

### 4.4. Бридинг
```rust
pub fn can_breed(a: &Pet, b: &Pet) -> BreedCheck;  // возраст, родство, кулдаун
pub fn breed(
    a: &Genome, b: &Genome, catalysts: &Catalysts, rng: &mut Rng,
) -> EggGenome;     // содержит геном + инкубационное время
pub fn mutation_chance(a: &Genome, b: &Genome, catalysts: &Catalysts) -> f32;
pub fn stat_cap_penalty(generation: u32) -> f32;
pub fn hybrid_of(e1: Element, e2: Element) -> Option<Element>;
```

### 4.5. Симбиоз (активность)
```rust
pub fn compute_goals(baseline: &PersonalBaseline) -> DailyGoals;
pub fn compute_vitality(snapshot: &DailyActivitySnapshot, goals: &DailyGoals) -> u16; // capped 150
pub fn compute_stat_gains(
    snapshot: &DailyActivitySnapshot,
    goals: &DailyGoals,
    genome: &Genome,
    streak_days: u32,
) -> StatGains;
pub fn synergy_multiplier(streak_days: u32) -> f32;
pub fn resonance_bonus(workout_kind: u8, element: Element) -> f32;
```

### 4.6. Бой
```rust
pub fn effective_power(loadout: &Loadout) -> u32;
pub fn simulate_combat(match_: &Match, seed: u64) -> MatchResult;
pub fn tech_card_bonus(card_type: TechniqueType, affinity: TechniqueType) -> f32;
pub fn element_multiplier(attacker: Element, defender: Element) -> f32;
```

### 4.7. Экономика
```rust
pub fn roll_gacha(banner: &Banner, rng: &mut Rng) -> GachaResult;
pub fn pity_progress(player: &PlayerPulls) -> PityState;
pub fn price_for(item: &ItemDef) -> Price;
```

### 4.8. Ошибки
```rust
#[repr(C)]
pub enum CoreError {
    Ok,
    InvalidInput,
    NotFound,
    ConstraintViolated,
    OutOfRange,
    RateLimited,
}
```

---

## 5. ИНВАРИАНТЫ (всегда истинны)

1. `0 ≤ needs.* ≤ 100` — все потребности в диапазоне.
2. `pet.level ≥ 1` всегда.
3. `Genome.generation` монотонно не убывает при breeding.
4. `quality_score` всегда возвращает `0..=100`.
5. `compute_vitality` всегда возвращает `0..=150`.
6. `simulate_combat` детерминирован: одинаковый `Match` + `seed` → одинаковый `MatchResult`.
7. `validate_heart` на Absent-пульсе → `heartScore = 0`.
8. `tech_card_bonus(card.type, pet.genome.tech_affinity)` даёт бонус только при совпадении.

---

## 6. СЕРИАЛИЗАЦИЯ

- Все persistent-типы (`Pet`, `Genome`, `TechniqueCard`, `DailyActivitySnapshot`) сериализуются в JSON или MessagePack.
- Каждое сохранение начинается с `schemaVersion: u32`. Миграции — в `serde_helpers.rs`.
- Бэквардная совместимость: ядро умеет читать сохранения предыдущей минорной версии.

---

## 7. ТЕСТЫ (обязательны)

### Unit-тесты
- Каждая public-функция ≥3 тестов (happy path, edge case, invalid input).

### Property-тесты (`proptest`)
- `simulate_combat` всегда детерминирован.
- `breed` всегда возвращает валидный `Genome`.
- `quality_score` всегда в `[0,100]`.
- `compute_vitality` всегда в `[0,150]`.
- `mutation_chance` всегда в `[0,1]`.

### Cross-platform тесты (golden tests)
- Один и тот же `Match` + `seed` → одинаковые `MatchResult` на Rust, iOS, Android, server.
- Зафиксированные JSON-файлы с expected-результатами в `tests/golden/`.

---

## 8. ПРИМЕР FFI (Rust → C header)

```rust
#[no_mangle]
pub extern "C" fn gochya_quality_score(
    metrics: *const PunchMetrics,
    heart: *const HeartRateEvidence,
) -> u8 {
    let m = unsafe { &*metrics };
    let e = unsafe { &*heart };
    quality_score(m, e)
}
```

`cbindgen` генерирует `gochya_core.h` → Swift/C# consume.

---

## 9. СВЯЗАННЫЕ ДОКУМЕНТЫ

- `CORE_FORMULAS.md` — числовые формулы, используемые в этих функциях.
- `docs/05-security/ANTICHEAT.md` — как сервер использует эти функции.
- `docs/03-architecture/ARCHITECTURE.md` — как ядро встроено в платформы.
