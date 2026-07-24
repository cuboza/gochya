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

Источник истины по таргетам — `ARCHITECTURE.md` §4. Здесь та же таблица:

| Таргет | Выход | Примечание |
|---|---|---|
| `aarch64-apple-ios` | `.a` → slice в `.xcframework` | iOS (companion) |
| **`arm64_32-apple-watchos`** | `.a` → slice в `.xcframework` | **Apple Watch, см. предупреждение ниже** |
| `aarch64-apple-ios-sim`, `x86_64-apple-ios` | slices в `.xcframework` | Apple Silicon и Intel simulator runners |
| `aarch64-linux-android` | `.so` | JNI для Kotlin-клиента (Wear OS) |
| `x86_64-unknown-linux-gnu` | `.a` → cgo | сервер |
| `x86_64-*` | native binary | CI/тесты |

> 🚨 **watchOS — самый рискованный таргет сборки, и это не `aarch64-apple-ios`.**
> Приложения watchOS для Apple Watch Series 4+ используют ABI **`arm64_32`**
> (64-битные регистры, 32-битные указатели). Соответствующий Rust-таргет —
> `arm64_32-apple-watchos`, и он **tier 3**: нет пребилда `std`, требуется
> nightly-toolchain с `-Z build-std`. Это надо проверить спайком в Sprint 0
> **до** того, как ядро вырастет: если таргет не заведётся, вариантами остаются
> C-обёртка через отдельный статический слой либо перенос части логики на Swift,
> и оба меняют архитектуру.
>
> `aarch64-apple-watchos` существует, но относится к симулятору и к arm64-моделям;
> для устройства нужен `arm64_32`. Прежняя строка «`aarch64-apple-ios` (watchOS/iOS)»
> объединяла две разные платформы с разным ABI.

> WASM (`wasm32-wasip1`) — опция для будущего sandboxing, **не для MVP**
> (`ARCHITECTURE.md` §4). Прежде здесь стоял `wasm32-unknown-unknown`, который
> не подходит для серверного исполнения (нет WASI-интерфейсов).

Альтернатива — C#/.NET 8 NativeAOT (если команда сильнее в C#). Учесть, что
NativeAOT под watchOS официально не поддерживается — этот путь закрывает Apple Watch.

---

## 3. ОСНОВНЫЕ ТИПЫ

### 3.1. Базовые enum'ы

> **Правило записи.** В Rust дискриминант задаётся атрибутом `#[repr(u8)]`, а не
> синтаксисом `enum X: u8` (это C#/Swift). Для FFI используется `#[repr(u8)]` на
> data-less enum'ах и `#[repr(C)]` на структурах.

```rust
#[repr(u8)]
pub enum Element {
    Fire = 0, Water = 1, Earth = 2, Air = 3,
    Light = 4, Dark = 5, Arcane = 6,
    // Гибриды (см. HYBRID_TABLE в BALANCE.md §2):
    Steam = 7, Magma = 8, Storm = 9, Mud = 10, Smoke = 11,
    Sand = 12, Eclipse = 13, Inferno = 14, Prism = 15, Crystal = 16,
}

#[repr(u8)]
pub enum Rarity {
    Common = 0, Uncommon = 1, Rare = 2, Epic = 3, Legendary = 4, Mythic = 5,
}

#[repr(u8)]
pub enum TechniqueType {
    Jab = 0, Hook = 1, Uppercut = 2, Cross = 3, Kick = 4, Elbow = 5, Block = 6,
}

#[repr(u8)]
pub enum Stat { Str = 0, Agi = 1, End = 2, Foc = 3 }

#[repr(u8)]
pub enum Stage { Egg = 0, Baby = 1, Teen = 2, Adult = 3, Premium = 4 }

#[repr(u8)]
pub enum Branch { Brute = 0, Adept = 1, Sage = 2 }
```

**`Effect` — плоская форма вместо enum с payload.** Rust-enum с данными
(`Stun(u8)`, `Crit(f32)`) **не является FFI-safe** и не может лежать внутри
`#[repr(C)]`-структур, которые маршалятся в Swift/C#/Dart. Поэтому тег и значение
разделены:

```rust
#[repr(u8)]
pub enum EffectKind {
    None = 0, Stun = 1, Bleed = 2, Crit = 3, Slow = 4, Heal = 5,
}

#[repr(C)]
pub struct Effect {
    pub kind:  EffectKind,
    pub value: f32,      // Stun/Slow — раунды; Bleed — число стеков;
                         // Crit — множитель крита; Heal — HP. Для None игнорируется.
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
    pub ability:       Ability,           // Ability::None вместо Option — FFI-safe
    pub generation:    u32,
}

/// Пассивная способность. `None` играет роль отсутствия — `Option<T>` не FFI-safe.
#[repr(u8)]
pub enum Ability {
    None = 0, Regen = 1, CritAura = 2, Thorns = 3,
    Shield = 4, Lifesteal = 5, LineageSignature = 6,
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
    pub last_bred_at: u64,         // 0 = не скрещивался; нужен для кулдауна 24ч (canBreed)
    pub needs_zero_since: u64,     // 0 = все потребности > 0; иначе ts начала «нуля».
                                   // Weakness наступает через 6ч — см. CORE_FORMULAS §1.6
    pub is_weak:      bool,
}

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
    pub crit_chance:  f32,   // 0..0.35; сохраняется, т.к. raw PunchMetrics
                             // не уходят с устройства и недоступны серверу в бою
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
    pub sleep_quality:   u8,                   // 0..100 — нужен для dailyGain[FOC]
    pub active_calories: u16,
    pub workouts:        [WorkoutSummary; MAX_WORKOUTS],  // фиксированный массив
    pub workout_count:   u8,                   // сколько элементов заполнено
    pub avg_hr:          u16,
    pub hr_zone_high_min: u16,                 // минуты в высокой ЧСС-зоне — dailyGain[AGI]
    pub meditation_min:  u16,                  // dailyGain[FOC]
    pub stress_level:    u8,                   // 0..100
    pub floors:          u16,
    pub stand_hours:     u8,
    pub source:          DataSource,
    pub timestamp:       u64,
}

pub const MAX_WORKOUTS: usize = 8;   // в зачёт идут первые MAX_WORKOUTS_FOR_GAIN = 3

#[repr(u8)]
pub enum DataSource { Watch = 0, Phone = 1 }

#[repr(C)]
pub struct WorkoutSummary {
    pub kind:      u8,    // стабильный WorkoutKind ID ниже
    pub duration_min: u16,
    pub calories:    u16,
}

#[repr(u8)]
pub enum WorkoutKind {
    Running = 0,
    Cycling = 1,
    Strength = 2,
    Swimming = 3,
    Yoga = 4,
    Meditation = 5,
    Hiit = 6,
    Other = 255,
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
    pub str: i16,   // знаковые: dailyGain[FOC] содержит вычитание `- stress/20`
    pub agi: i16,   // и может быть отрицательным. При u16 это переполнение.
    pub end: i16,
    pub foc: i16,
}
```

### 3.6. Бой (авто-баттлер)
```rust
#[repr(C)]
pub struct Loadout {
    pub pet_id:        [u8; 16],
    pub pet_stats:     Stats,
    pub pet_genome:    Genome,
    pub pet_mood:      u8,                  // 0..100 — нужен для moodMultiplier
                                            // в боевой формуле (CORE_FORMULAS §5.2).
                                            // Без него simulate_combat её не вычислит.
    pub cards:         [TechniqueCard; 5],
    pub signature_idx: u8,                  // 0..4 — какая из карт занимает ult-слот
    pub gear:          GearSummary,
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

#[repr(u8)]
pub enum MatchMode { Casual = 0, Ranked = 1, Tournament = 2 }

pub const MAX_ROUNDS: usize = 20;   // предел боя, см. CORE_FORMULAS §5.4

#[repr(C)]
pub struct MatchResult {
    pub winner:      Winner,
    pub rounds:      [RoundLog; MAX_ROUNDS],  // массив, а не одиночная структура
    pub round_count: u8,                      // сколько элементов заполнено
    pub final_hp_a:  u16,
    pub final_hp_b:  u16,
    pub seed:        u64,
}

#[repr(u8)]
pub enum Winner { A = 0, B = 1, Draw = 2 }

#[repr(C)]
pub struct RoundLog {
    pub card_a_idx: u8,
    pub card_b_idx: u8,
    pub damage_a_to_b: u16,
    pub damage_b_to_a: u16,
    pub effect_triggered: Effect,
}
```

### 3.7. Служебные типы public API

Эти типы фигурируют в сигнатурах §4. Без них контракт не компилируется.

```rust
/// Детерминированный ГПСЧ. Основа гарантии «одинаковые входы → одинаковые выходы».
/// Алгоритм фиксирован (PCG-XSH-RR 64/32) и НЕ может быть заменён на системный:
/// от него зависит воспроизводимость боёв на всех платформах и golden-тесты.
#[repr(C)]
pub struct Rng { pub state: u64, pub inc: u64 }

#[repr(C)]
pub struct EvolutionCheck {
    pub can_evolve:      bool,
    pub next_stage:      Stage,
    pub missing_level:   u32,    // 0 = требование выполнено
    pub missing_mood:    u8,
    pub missing_stats:   u32,
}

/// Единственное определение. Раньше этот enum задавался дважды (здесь и в §4.2)
/// с разными вариантами — `Hug`/`Medicine` против `Cure`/`UseItem`.
#[repr(u8)]
pub enum CareAction {
    Feed = 0,      // требует item_def_id (еда)
    Clean = 1,     // требует item_def_id (мыло/шампунь) либо 0 для мини-игры
    Play = 2,
    Sleep = 3,
    Hug = 4,       // быстрый +mood по тапу на питомца
    Cure = 5,      // снятие Weakness, требует item_def_id (лекарство)
}

#[repr(C)]
pub struct HeartVerdict {
    pub passed:      bool,
    pub heart_score: f32,        // 0.0 либо 0.5..0.70 — см. CORE_FORMULAS §2.2
    pub reason:      HeartFailReason,
}

/// Причина отказа — для человекочитаемого сообщения на часах, а не только для лога.
#[repr(u8)]
pub enum HeartFailReason {
    Ok = 0, LowPresence = 1, NoElevation = 2, TooLow = 3, PoorContact = 4,
}

#[repr(C)]
pub struct BreedCheck {
    pub can_breed:        bool,
    pub reason:           BreedFailReason,
    pub cooldown_left_s:  u64,
    pub inbreeding_coeff: u8,
}

#[repr(u8)]
pub enum BreedFailReason {
    Ok = 0, NotAdult = 1, LowLevel = 2, TooRelated = 3, OnCooldown = 4,
    MissingCatalyst = 5, IsWeak = 6,
}

#[repr(C)]
pub struct Catalysts { pub love_crystal: bool, pub mutation: bool }

#[repr(C)]
pub struct EggGenome {
    pub genome:           Genome,
    pub incubation_hours: u8,    // 4..24, см. CORE_FORMULAS §3.2
    pub parent_a:         [u8; 16],
    pub parent_b:         [u8; 16],
    pub mutated_genes:    u16,   // битовая маска мутировавших генов — для родословной
}

#[repr(C)]
pub struct Banner {
    pub id:          [u8; 16],
    pub kind:        BannerKind,
    pub pool_len:    u16,
    pub pool:        *const ItemDef,   // владелец памяти — вызывающая сторона
}

#[repr(u8)]
pub enum BannerKind { Standard = 0, Premium = 1 }

#[repr(C)]
pub struct GachaResult {
    pub item:        ItemDef,
    pub rarity:      Rarity,
    pub from_pity:   bool,       // сработала ли гарантия — показывается игроку
}

#[repr(C)]
pub struct PlayerPulls {
    pub since_rare: u16,
    pub since_epic: u16,
    pub total:      u32,
}

#[repr(C)]
pub struct PityState {
    pub pulls_to_rare: u16,
    pub pulls_to_epic: u16,
}

#[repr(C)]
pub struct ItemDef {
    pub item_def_id: u32,   // каталожный номер — тот же тип, что InventoryEntry.item_def_id
                            // и аргумент apply_care_action(). НЕ [u8;16]: определение
                            // предмета адресуется стабильным числом каталога, а UUID
                            // получают только инстансы в инвентаре.
    pub kind:        ItemKind,
    pub rarity:      Rarity,
    pub stats:       GearSummary,
}

#[repr(u8)]
pub enum ItemKind {
    Food = 0, Potion = 1, Cosmetic = 2, Gear = 3,
    Egg = 4, Catalyst = 5, LoveCrystal = 6, Decor = 7,
}

#[repr(C)]
pub struct Price { pub currency: Currency, pub amount: u32 }

#[repr(u8)]
pub enum Currency { Koins = 0, Gems = 1, Vitality = 2, Crowns = 3 }

// audit V1: Player/Wallet/Inventory как core-типы (не только SQL)
#[repr(C)]
pub struct Player {
    pub id:            [u8; 16],
    pub display_name:  [u8; 32],
    pub created_at:    u64,
    pub timezone:      [u8; 32],   // IANA tz string, null-padded
    pub active_pet_id: [u8; 16],
    pub streak_days:   u32,        // для synergy multiplier (раньше было «негде хранить»)
}

#[repr(C)]
pub struct Wallet {
    pub player_id:       [u8; 16],
    pub koins:           u64,
    pub gems:            u64,
    pub vitality_daily:  u32,   // обнуляется при смене суток
    pub vitality_date:   u32,   // unix day (ts / 86400)
    pub crowns:          u32,
}

// Inventory — коллекция стеков; ядро оперирует массивом, сервер хранит в SQL
#[repr(C)]
pub struct InventoryEntry {
    pub item_def_id: u32,
    pub quantity:    u32,
    pub metadata:    [u8; 32],  // для equipped/pet binding и пр.
}

#[repr(C)]
pub struct Inventory {
    pub player_id: [u8; 16],
    pub entries:   *const InventoryEntry,
    pub entries_len: u32,
}

// audit V7: турнирный bracket
#[repr(C)]
pub struct Bracket {
    pub tournament_id: [u8; 16],
    pub round:         u8,
    pub slot_a:        [u8; 16],   // player_id или нули
    pub slot_b:        [u8; 16],
    pub winner:        [u8; 16],   // нули пока не сыграно
    pub match_id:      [u8; 16],   // ссылка на Match
}
```

---

## 4. PUBLIC API (Rust domain API)

Сигнатуры ниже — удобный Rust domain API. Внешний C ABI не экспортирует их
напрямую: он использует versioned POD-структуры, `GochyaStatus` и out-параметры
из `CORE_ABI.md`. Это исключает Rust references и возврат крупных структур
by-value на границе Swift/Kotlin/Dart/Go.

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

// audit C2: расширенный API ухода + эволюции
pub fn apply_xp(pet: &mut Pet, xp_gained: u64) -> LevelUpResult; // накапливает XP, повышает уровень
pub fn apply_decay_since(pet: &mut Pet, now_unix: u64) -> CoreError; // офлайн-reconciliation: применяет decay с pet.last_sync до now
pub fn set_sleeping(pet: &mut Pet, sleeping: bool, until_unix: u64);

// Границу FFI пересекают только C-совместимые типы: код ошибки вместо Result,
// флаг вместо Option. Внутри ядра можно пользоваться идиоматичным Rust.
pub fn evolve(pet: &mut Pet, branch_hint: Branch, use_hint: bool) -> CoreError;
pub fn apply_care_action(pet: &mut Pet, action: CareAction, item_def_id: u32) -> CoreError;
//   item_def_id: ссылка на предмет (еда/мыло/зелье) — таблица эффектов в economy.rs
//   (audit C2: без item_id логика еды/расходников была не привязана)
```

> `CareAction` определён в §3.7 — единственное место. Дублирующее определение
> с вариантами `Cure`/`UseItem` отсюда удалено: два несовпадающих объявления
> одного enum'а в одном файле контракта означали, что клиенты и сервер
> маршалили бы разные числовые коды для одного действия.

### 4.3. Dojo / Technique Card
```rust
pub fn validate_heart(e: &HeartRateEvidence) -> HeartVerdict;
pub fn spirit_bonus(e: &HeartRateEvidence) -> f32;
pub fn quality_score(m: &PunchMetrics, e: &HeartRateEvidence) -> u8; // 0..100
pub fn rarity_from_quality(q: u8) -> Rarity;
pub fn create_technique_card(
    m: &PunchMetrics, e: &HeartRateEvidence, owner: &[u8;16], ts: u64, rng: &mut Rng,
) -> TechniqueCard;

// audit C2: helper-функции, используемые в quality_score (CORE_FORMULAS §2)
pub fn norm_power(peak_accel_mps2: f32) -> f32;             // → [0, 1]
pub fn combo_score(combo_len: u8) -> f32;                   // → [0, 1]
pub fn heart_score(e: &HeartRateEvidence) -> f32;           // → [0, 0.70] или 0
pub fn muscle_memory_bonus(repeat_count_of_type: u32) -> f32; // → [0, 0.15]
```

### 4.4. Бридинг
```rust
pub fn can_breed(a: &Pet, b: &Pet) -> BreedCheck;  // возраст, родство, кулдаун
pub fn breed(
    a: &Genome, b: &Genome, catalysts: &Catalysts, rng: &mut Rng,
) -> EggGenome;     // содержит геном + инкубационное время
pub fn mutation_chance(a: &Genome, b: &Genome, catalysts: &Catalysts) -> f32;
pub fn stat_cap_penalty(generation: u32) -> f32;
pub fn hybrid_of(e1: Element, e2: Element) -> Element;   // Element::None-эквивалента нет:
                                                          // возвращает e1 при отсутствии гибрида

/// Коэффициент родства: число общих предков в пределах глубины `depth`.
/// Используется в `can_breed` (порог ≤3) и в `mutation_chance` (штраф −0.02·coeff).
/// Требует родословной, поэтому принимает предков явно — ядро не ходит в БД.
pub fn inbreeding_coeff(
    lineage_a: *const [u8; 16], len_a: u16,
    lineage_b: *const [u8; 16], len_b: u16,
) -> u8;
```

### 4.5. Симбиоз (активность)
```rust
pub fn compute_goals(baseline: &PersonalBaseline) -> DailyGoals;
pub fn compute_vitality(
    snapshot: &DailyActivitySnapshot, goals: &DailyGoals, streak_days: u32,
) -> u16; // capped 150; streak_days обязателен — см. CORE_FORMULAS.md §4.3
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

// audit C2: недостающие функции из CORE_FORMULAS §5
pub fn overlevel_penalty(level: u32) -> u32;
pub fn defense_ratio(foc_stat: u32, gear_foc_bonus: i16) -> f32;
pub fn starting_stamina(end_stat: u32) -> u32;
pub fn stamina_regen(end_stat: u32) -> u32;
pub fn rng_variance(rng: &mut Rng, lo: f32, hi: f32) -> f32;

/// Выбор карты. Помимо динамического состояния нужны сами карты и геном —
/// эвристика в CORE_FORMULAS §5.2-extended обращается к baseDamage, типу карты,
/// tech_affinity и стихиям обеих сторон. Через `CombatantState` они недоступны,
/// поэтому лоадауты передаются явно.
pub fn select_card_ai(
    my_loadout:    &Loadout,
    my_state:      &CombatantState,
    enemy_loadout: &Loadout,
    enemy_state:   &CombatantState,
) -> u8;   // index в my_loadout.cards, 0..4

pub fn apply_active_effects(state: &mut CombatantState) -> Effect; // bleed tick, stun decrement
```

#### CombatantState (внутреннее состояние симуляции, audit C1)

Только то, что **меняется по ходу боя**. Статика (карты, геном, снаряжение) живёт
в `Loadout` и передаётся в функции отдельно.

```rust
#[repr(C)]
pub struct CombatantState {
    pub hp:            i32,
    pub stamina:       i32,
    pub active:        ActiveEffects,
    pub signature_cd:  u8,    // раунды до готовности signature
}

#[repr(C)]
pub struct ActiveEffects {
    pub stun_rounds:   u8,
    pub bleed_stacks:  u8,
    pub slow_rounds:   u8,
}
```

Для MVP `Bleed` наносит `8 HP` за стек в начале каждого следующего раунда и
сохраняется до конца боя. `Slow` уменьшает initiative на `20` на указанное число
раундов. Эти числа входят в golden path и меняются только вместе с
`CORE_FORMULAS.md` и golden fixtures.

### 4.7. Экономика
```rust
pub fn roll_gacha(banner: &Banner, rng: &mut Rng) -> GachaResult;
pub fn pity_progress(player: &PlayerPulls) -> PityState;
pub fn price_for(item: &ItemDef) -> Price;
```

### 4.8. Ошибки
```rust
#[repr(u8)]
pub enum CoreError {
    Ok = 0,
    InvalidInput = 1,
    NotFound = 2,
    ConstraintViolated = 3,
    OutOfRange = 4,
    RateLimited = 5,
}
```

---

## 5. ИНВАРИАНТЫ (всегда истинны)

> Каждый инвариант обязан существовать как **исполняемый тест** в `core/tests/`,
> а не только как утверждение в этом файле. Инварианты, проверяемые лишь чтением
> документа, расходятся с кодом — именно так возникло нарушение №5.

1. `0 ≤ needs.* ≤ 100` — все потребности в диапазоне.
2. `pet.level ≥ 1` всегда.
3. `Genome.generation` монотонно не убывает при breeding.
4. `quality_score` всегда возвращает `0..=100`.
5. `compute_vitality` всегда возвращает `0..=150`.
   **Держится на порядке операций в `CORE_FORMULAS.md` §4.3:** clamp применяется
   после умножения на `synergyMultiplier`. При обратном порядке максимум = 205.5.
6. `simulate_combat` детерминирован: одинаковый `Match` + `seed` → одинаковый `MatchResult`.
7. `validate_heart` на Absent-пульсе → `heartScore = 0`.
8. `tech_card_bonus(card.type, pet.genome.tech_affinity)` даёт бонус только при совпадении.
9. `round_count ≤ MAX_ROUNDS`, `workout_count ≤ MAX_WORKOUTS`, `signature_idx ≤ 4` —
   длины не выходят за границы своих массивов.
10. **Баланс стихий.** Для каждого `Element` выполняется одновременно:
    (а) существует контра — `∃x: elementMultiplier(x, e) > 1.0`;
    (б) нет слабого доминирования — неверно, что строка `e` поэлементно ≥ столбца `e`
    при хотя бы одном строгом превосходстве.
    Без этого мета схлопывается к доминирующей стихии.

    **Проверяется на каждом релизном подмножестве стихий, а не только на полном наборе.**
    Тест обязан прогонять как минимум: `{Fire,Water,Earth}` (MVP), `{Fire,Water,Earth,Steam}`,
    `{Fire,Water,Earth,Air}`, полный набор. Инвариант, проверенный только на полном
    наборе, пропускает дефект: круг из 4 звеньев разваливался при выпуске MVP без Air,
    оставляя Water без контры. Текущие разбросы — в `BALANCE.md` §1.2.

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

## 8. ПРИМЕР FFI (Rust → C header) — с panic safety

> ⚠️ **Аудит D4 (критично):** прежний пример `gochya_quality_score` не содержал `catch_unwind`, что прямо нарушает принцип §1.6 «Без паник в FFI». Паника через FFI крашит весь процесс (на часах = краш приложения). **Все** `extern "C"` обёртки обязаны оборачивать тело в `std::panic::catch_unwind`.

```rust
use std::panic;

#[no_mangle]
pub extern "C" fn gochya_quality_score(
    metrics: *const PunchMetrics,
    heart: *const HeartRateEvidence,
) -> u8 {
    if metrics.is_null() || heart.is_null() {
        return 0; // безопасный дефолт
    }
    let result = panic::catch_unwind(|| {
        let m = unsafe { &*metrics };
        let e = unsafe { &*heart };
        quality_score(m, e)
    });
    match result {
        Ok(score) => score,
        Err(_) => {
            // логируем в anticheat_events / crash reporter (через set_hook в lib init)
            0 // безопасный дефолт при панике
        }
    }
}
```

### Инициализация (один раз при загрузке библиотеки)
```rust
#[no_mangle]
pub extern "C" fn gochya_core_init() -> CoreError {
    panic::set_hook(Box::new(|info| {
        // отправить в crash reporter хоста (iOS: UIAlertController-free logging,
        // Android: Log.e, server: tracing)
        log_panic_to_host(&info.to_string());
    }));
    CoreError::Ok
}
```

### Передача больших структур через out-параметр (audit D3/T4)
Для `MatchResult` и других крупных возвратов — **не** возвращать by value
(нестандартизированный ABI между ARM64/x86_64). Реализованный compact combat
wire contract использует caller-owned out-параметр:
```rust
#[unsafe(no_mangle)]
pub extern "C" fn gochya_simulate_combat_v1(
    match_: *const GochyaCombatMatchV1,
    seed: u64,
    out_result: *mut GochyaCombatResultV1,
) -> GochyaStatus;
```

`GochyaCombatMatchV1` имеет `struct_size/schema_version` и содержит только поля,
которые реально читает боевая формула. Результат содержит фиксированные 20
round slots и `round_count`; полный layout и размеры нормативно зафиксированы в
`CORE_ABI.md` §11.

`cbindgen` генерирует `gochya_core.h` → Swift / JNI (Kotlin) consume. Для iOS символьный lookup через `DynamicLibrary.process()` (см. `CLIENT_COMPANION.md §6`).

---

## 9. СВЯЗАННЫЕ ДОКУМЕНТЫ

- `CORE_FORMULAS.md` — числовые формулы, используемые в этих функциях.
- `docs/05-security/ANTICHEAT.md` — как сервер использует эти функции.
- `docs/03-architecture/ARCHITECTURE.md` — как ядро встроено в платформы.
