# SHARED CORE — Формулы (единый источник истины)

> Все числа баланса в одном месте. **Любой код, использующий эти формулы, вызывает их через Shared Core, а не пересчитывает локально.** Числа — стартовые; корректируются через A/B-тесты.

---

## 1. ПОТРЕБНОСТИ И ЭВОЛЮЦИЯ

### 1.1. Decay потребностей (за секунду)
```
HUNGER_DECAY_PER_HOUR   = 1.0     // −1/час
ENERGY_DECAY_PER_HOUR   = 0.67    // ~−1/1.5ч
HYGIENE_DECAY_PER_HOUR  = 0.5     // ~−1/2ч
MOOD_DECAY_PER_HOUR     = depends on other needs (см. ниже)

NIGHT_DECAY_MULTIPLIER  = 0.333   // ночью в 3× медленнее
```

### 1.2. Mood decay (зависит от остальных)
```
mood_decay = avg(0, hunger==0 ? 1 : 0,
                  energy==0 ? 1 : 0,
                  hygiene==0 ? 1 : 0) * MOOD_DECAY_RATE
```
- Если все потребности > 0 → mood не падает.
- Если любая = 0 → mood −1/2час.

### 1.3. Mood multiplier
```
moodMultiplier(mood) = 0.7 + 0.3 * (mood / 100)    // [0.7 .. 1.0]
allStatGains *= moodMultiplier
```

### 1.4. XP curve
```
xpToNext(level) = floor(80 * level^1.5)
// Lv1→2: 80, Lv5→6: 894, Lv10→11: 2529, Lv20→21: 7155, Lv30→31: 13145
```
> ⚠️ **Кривая XP пока не согласована с таблицей прогрессии** `BALANCE.md` §9.
> Обратный расчёт по этой формуле требует немонотонного темпа 702 → 9 963 → 2 050 XP/день,
> а таблицы «действие → XP» в документации не существует. До её появления
> `BALANCE.md` §9 — это пожелание, а не спецификация. См. план исправления, B3.

### 1.5. Эволюция
```
canEvolve(pet):
    return pet.level >= THRESHOLD_FOR_STAGE[pet.stage+1]
        && pet.needs.mood >= 50
        && pet.stats.sum() >= MIN_STATS_FOR_STAGE[pet.stage+1]

THRESHOLD_FOR_STAGE:
    Egg→Baby:    нет (время инкубации)
    Baby→Teen:   level 10
    Teen→Adult:  level 30
    Adult→Premium: level 50 + спец-условие
```

### 1.6. Neglect / Weakness
```
enterWeakness(pet):
    if any(needs.* == 0) for 6 consecutive hours → Weakness
    while in Weakness: pet.xp_gains = 0, cannot PvP, cannot breed
exitWeakness:
    when all needs.* >= 50 again
```

---

## 2. DOJО / TECHNIQUE CARD

### 2.1. Heart Gate (валидация пульса)
```
heartGate(heart) =
    (heart.present     >= 0.80)
    AND (heart.mean    >= heart.baseline + 8)
    AND (heart.mean    >= 55)
    AND (heart.confidence >= 0.85)
```

### 2.2. Spirit Bonus
```
spiritBonus(heart) = clamp((heart.delta - 8) / 40, 0, 0.20)   // [0 .. 0.20]
heartScore(heart)  = heartGate(heart) ? (0.5 + spiritBonus(heart)) : 0    // [0.5 .. 0.70] или 0
```

### 2.3. Normalized power
```
normPower(peakAccel) = clamp((peakAccel - 20) / 90, 0, 1)   // 20 м/с² → 0, 110 м/с² → 1
```

### 2.4. Quality Score (0..100)
```
qualityScore(metrics, heart) = 100 * (
      0.40 * normPower(metrics.peak_accel)
    + 0.25 * metrics.precision
    + 0.12 * comboScore(metrics.combo_len)
    + 0.08 * metrics.rhythm_score
    + 0.15 * heartScore(heart)
)

comboScore(comboLen) = clamp(comboLen / 5, 0, 1)   // 0..5+ ударов → 0..1
```

### 2.5. Rarity from quality
```
rarityFromQuality(q):
    if q < 40:   Common
    elif q < 55: Uncommon
    elif q < 70: Rare
    elif q < 85: Epic
    elif q < 95: Legendary
    else:        Mythic
```

### 2.6. Technique Card stats
```
typeMultiplier(techType) = из таблицы (см. BALANCE.md), обычно 1.0 ± 0.2
baseDamage = (peakAccel / 50) * precision * typeMultiplier * techLevel
            // techLevel — уровневый множитель, растёт от игрока (1.0..1.5)
speed       = 100 / (1 + execTime_seconds)
staminaCost = round((peakAccel / 50) * 2.2)
critChance  = clamp(0.02 + 0.01 * comboLen + 0.05 * (rhythmScore - 0.5), 0, 0.35)
```

### 2.7. Мышечная память (антигринд-бонус)
```
muscleMemoryBonus(repeatCount) = clamp(repeatCount / 50 * 0.01, 0, 0.15)   // кап +15%
```

### 2.8. Дневные лимиты Dojo
```
DOJO_DAILY_LIMIT       = 10
DOJO_DAILY_PREMIUM_SLOTS = 5    // за GOCHYA+
```

---

## 3. БРИДИНГ

### 3.1. Условия скрещивания
```
canBreed(a, b):
    return a.stage == Adult
        && b.stage == Adult
        && a.level >= 30
        && b.level >= 30
        && inbreedingCoeff(a, b) <= 3           // поколения
        && a.last_bred_at + 24h <= now
        && b.last_bred_at + 24h <= now
```

### 3.2. Инкубация
```
INCUBATION_MIN_HOURS = 4
INCUBATION_MAX_HOURS = 24
incubationHours(rng) = rng_range(4, 24)
```

### 3.3. Наследование генов
```
for each gene g in visual/stats/element/techAffinity/ability:
    child.g = random_choice([a.g, b.g])   // 50/50

for each stat_potential sp:
    lo = min(a.sp, b.sp) * 0.95
    hi = max(a.sp, b.sp) * 1.05
    child.sp = rng_range(lo, hi)

child.generation = max(a.generation, b.generation) + 1
```

### 3.4. Шанс мутации
```
mutationChance(a, b, catalysts) =
      0.04
    + 0.01 * avgParentRarity
    + 0.05 * (a.element != b.element ? 1 : 0)
    + 0.10 * (catalysts.mutation ? 1 : 0)
    - 0.02 * inbreedingCoeff(a, b)
    clamp to [0, 0.30]
```
- Применяется к каждому гену независимо с шансом `mutationChance`.

### 3.5. Гибридные стихии
```
hybridOf(e1, e2):
    if (e1, e2) in HYBRID_TABLE: return Some(HYBRID_TABLE[(e1, e2)])
    else: return None

HYBRID_TABLE (часть, полная в BALANCE.md):
    (Fire, Water)   → Steam
    (Fire, Earth)   → Magma
    (Air,  Water)   → Storm
    (Earth, Water)  → Mud
    ...
```
- Гибрид происходит с шансом `0.20` если родители разных стихий.

### 3.6. Штраф поколения
```
statCapPenalty(generation) = max(0, generation - 5) * 0.03   // -3% за gen сверх 5
effectiveStatCap = baseCap * (1 - statCapPenalty)
```

### 3.7. Связь с техникой
```
techCardBonus(cardType, affinity):
    return cardType == affinity ? 0.15 : 0   // +15% урон/скорость
```

---

## 4. СИМБИОЗ (АКТИВНОСТЬ)

### 4.1. Адаптивные цели
```
baselineSteps = movingAverage(steps, 14d)
dailyGoalSteps = clamp(baselineSteps * 1.15, 2500, 18000)

baselineSleep = movingAverage(sleep_hours, 14d)
dailyGoalSleep = clamp(baselineSleep * 1.10, 6, 9)

baselineCals = movingAverage(active_cals, 14d)
dailyGoalCals = clamp(baselineCals * 1.15, 200, 800)
```

### 4.2. Нормализации
```
stepsNorm      = clamp(steps / dailyGoalSteps, 0, 1.5)
sleepNorm      = clamp(sleepHours / dailyGoalSleep, 0, 1.3)
activeCalsNorm = clamp(activeCals / dailyGoalCals, 0, 1.5)
workoutBonus   = min(workoutCount, 3) * 0.3
```

### 4.3. Vitality (главная «здоровая» валюта)
```
base = 100 * (
      0.40 * stepsNorm
    + 0.25 * activeCalsNorm
    + 0.20 * sleepNorm
    + 0.15 * workoutBonus
)
vitality = clamp(base * synergyMultiplier(streak_days), 0, MAX_VITALITY_PER_DAY)
```
> **Порядок операций критичен.** Clamp применяется **после** умножения на стрик, иначе инвариант
> `CORE_SPEC.md` §5.7 (`compute_vitality ∈ [0, 150]`) нарушается: максимум базы = 137,
> после ×1.5 = 205.5. При правильном порядке: без стрика потолок 137 (кэп не связывает),
> со стриком 30 дней — ровно 150. Средний игрок (нормы 0.9/0.9/0.9, 1 тренировка)
> со стриком получает 121.5 — то есть стрик ценен для обычного игрока, а не только
> для экстремальной активности.

### 4.4. Pet Vitals (статы напрямую от активности)
```
dailyGain[STR] = floor((strengthWorkoutMin/30) * 5 + (floors/10) * 2)
dailyGain[AGI] = floor((cardioMin/30) * 5 + (hrZoneHighMin/10) * 2)
dailyGain[END] = floor((workoutDurationMin/60) * 5 + streakBonus)
dailyGain[FOC] = floor((meditationMin/15) * 3 + (sleepQuality/100) * 5 - (stress/20))

streakBonus = min(streak_days * 0.2, 3)
```

### 4.5. Synergy multiplier (стрик)
```
synergyMultiplier(streak_days) =
    if streak_days < 7: 1.0
    else: clamp(1.0 + (streak_days - 7) * 0.02, 1.0, 1.5)   // 30+ дней → 1.5
```

### 4.6. Resonance (совпадение стихии)
```
resonanceBonus(workoutKind, element):
    // таблица: напр. Running + Fire = +10%
    return RESONANCE_TABLE[(workoutKind, element)] ?? 0   // обычно 0..0.10
```

### 4.7. Базовый рост без активности
```
Базовые gains (без activity snapshot) = TARGET_GAINS * 0.60
То есть прогресс без спорта = 60% от максимума.
```

### 4.8. Дневные лимиты
```
MAX_VITALITY_PER_DAY = 150
MAX_WORKOUTS_FOR_GAIN = 3   // 4-я и далее тренировка не даёт gains
```

---

## 5. БОЙ (АВТО-БАТТЛЕР)

### 5.1. Effective Power (для матчмейкинга)
```
effectivePower(loadout) =
      sum(loadout.pet.stats) * 10
    + sum(card.baseDamage * RARITY_STAT_MULT[card.rarity] for card in loadout.cards)
    + sum(loadout.gear.bonuses) * 5
    - overlevelPenalty(loadout.pet.level)

overlevelPenalty(level) = max(0, level - 50) * 5
```
> ⚠️ **`RARITY_STAT_MULT` (1.0…2.5), не `GACHA_WEIGHTS`.** Это две разные таблицы в
> `BALANCE.md` §5, и их нельзя путать. `GACHA_WEIGHTS` (Common=100 … Mythic=1) — веса
> **выпадения** в гаче; подстановка их сюда делает колоду из 5 Common в 40 раз «сильнее»
> колоды из 5 Mythic и инвертирует матчмейкинг, на котором держится обещание
> «платное снаряжение не даёт преимущества».

### 5.2. Боевой раунд
```
// в начале боя:
hpA = 1000 + loadout_a.pet.stats.end * 10
hpB = 1000 + loadout_b.pet.stats.end * 10

// каждый раунд обе стороны выбирают карту (по AI-эвристике):
initiativeA = loadout_a.pet.stats.agi + card_a.speed
initiativeB = loadout_b.pet.stats.agi + card_b.speed
attacker = initiativeA > initiativeB ? A : B   // при равенстве — rng

damage = baseDamage
    * elementMultiplier(attacker.element, defender.element)
    * (1 + techCardBonus(card.type, attacker.genome.tech_affinity))
    * moodMultiplier(attacker.mood)
    * rngVariance(rng, [0.9, 1.1])
    * (1 - defender.defense_from_stats)

if rng < critChance: damage *= 1.8
defender.hp -= damage

apply effect (stun/bleed/etc.)
```

### 5.3. Element multiplier (камень-ножницы-бумага)

```
elementMultiplier(atk, def):
    if isHybrid(atk) or isHybrid(def):
        return average over базовых стихий-родителей
    return ELEMENT_TABLE[atk][def]      // прямое чтение таблицы
```

**Только чтение таблицы.** Прежний псевдокод ветвился по трём значениям
{1.0, 1.5, 0.67} и физически не мог выразить множители тир-треугольника (1.1 / 0.91),
из-за чего расходился с `BALANCE.md`. Единственный источник значений — `BALANCE.md` §1.

Структура таблицы (сами числа — в `BALANCE.md` §1):
```
1. Круг базовых:      Fire > Earth > Air > Water > Fire     (1.5 / 0.67)
2. Light <> Dark:     взаимно 1.5 (симметрично, баланс-нейтрально)
3. Треугольник тиров: базовые > Arcane > Light/Dark > базовые  (1.1 / 0.91)
```

> **Инвариант:** у каждой стихии есть контра, ни одна не доминирует
> (`CORE_SPEC.md` §5 №10). Проверяется тестом над таблицей, а не глазами.

### 5.4. Завершение боя
```
rounds_played: до 20 раундов или пока чей-то HP <= 0
winner = A if hpB <= 0 else B if hpA <= 0 else по оставшемуся HP (или Draw)
```

---

## 6. ЭКОНОМИКА

### 6.1. Награды за дуэль (ранговый режим)
```
WIN_REWARD_KOINS = 50 + rating_diff_bonus
LOSS_REWARD_KOINS = 15
DAILY_FIRST_WIN_BONUS = 100

rating_delta(winnerRating, loserRating):
    expected = 1 / (1 + 10^((loser - winner) / 400))
    K = matches_count < 30 ? 32 : 24
    return K * (1 - expected)   // добавляется победителю, вычитается у проигравшего
```

### 6.2. Гача-дропы
Конкретные проценты по каждому баннеру — **только** в `docs/05-security/BALANCE.md` §6 (`GACHA DROP_TABLES`), это единственный источник истины для чисел дропа. Здесь — только структура pity, общая для всех баннеров:
```
PITY:
    rare_pity_threshold = 10    // после 10 pull'ов без Rare+ — гарантия Rare+
    epic_pity_threshold = 50    // после 50 — гарантия Epic+
```

### 6.3. Цены (пример, корректируется в BALANCE.md)
```
PULL_COST_GEMS         = 100
PULL_COST_GEMS_10x     = 900 (скидка)
EGG_INCUBATE_SKIP_GEMS = 50
BREED_COST_KOINS       = 500
BREED_LOVE_CRYSTAL     = 1
```

### 6.4. ELO / лиги
```
Bronze:   0..1199
Silver:   1200..1499
Gold:     1500..1799
Platinum: 1800..2099
Diamond:  2100..2399
Master:   2400+

SEASON_RATING_SQUISH = 0.75   // в конце сезона rating *= 0.75
```

---

## 7. СЕЗОНЫ

```
SEASON_DURATION_DAYS = 28
SEASON_REWARDS_BY_LEAGUE:
    Bronze:   50 Crowns
    Silver:   100 Crowns
    Gold:     200 Crowns + косметика
    Platinum: 350 Crowns + косметика
    Diamond:  500 Crowns + эпический питомец
    Master:   1000 Crowns + легендарный питомец
```

---

## 8. УВЕДОМЛЕНИЯ

```
MAX_PUSH_PER_DAY = 3
PRIORITIES:
    1 (highest): egg_hatched, friend_challenge
    2: daily_goal_reached, season_ending
    3 (lowest): needs_decay_reminder
```

---

## 9. СВЯЗАННЫЕ ДОКУМЕНТЫ

- `CORE_SPEC.md` — сигнатуры функций для этих формул.
- `docs/05-security/BALANCE.md` — расширенные таблицы (ELEMENT_TABLE, HYBRID_TABLE, RESONANCE_TABLE, дроп-таблицы).
- `docs/02-mechanics/*` — контекст формул по механикам.
