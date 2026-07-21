# БАЛАНС — таблицы и числа

> Расширенные таблицы для формул из `docs/04-core/CORE_FORMULAS.md`. Стартовые значения; корректируются через A/B-тесты.

---

## 1. ELEMENT_TABLE (камень-ножницы-бумага)

| Атакующий ↓ / Защитник → | Fire | Water | Earth | Air | Light | Dark | Arcane |
|---|---|---|---|---|---|---|---|
| **Fire** | 1.0 | 0.67 | 1.5 | 1.0 | 1.0 | 1.0 | 0.8 |
| **Water** | 1.5 | 1.0 | 0.67 | 1.0 | 1.0 | 1.0 | 0.8 |
| **Earth** | 0.67 | 1.5 | 1.0 | 0.67 | 1.0 | 1.0 | 0.8 |
| **Air** | 1.0 | 1.0 | 1.5 | 1.0 | 1.0 | 1.0 | 0.8 |
| **Light** | 1.0 | 1.0 | 1.0 | 1.0 | 1.0 | 1.5 | 0.8 |
| **Dark** | 1.0 | 1.0 | 1.0 | 1.0 | 1.5 | 1.0 | 0.8 |
| **Arcane** | 1.2 | 1.2 | 1.2 | 1.2 | 1.2 | 1.2 | 1.0 |

**Гибриды** берут среднее от базовых стихий.

---

## 2. HYBRID_TABLE

| Родитель A | Родитель B | Гибрид | Особенность |
|---|---|---|---|
| Fire | Water | **Steam** | +доп. урон по Air |
| Fire | Earth | **Magma** | +броня |
| Air | Water | **Storm** | +скорость |
| Earth | Water | **Mud** | +замедление |
| Fire | Air | **Smoke** | +уклонение |
| Earth | Air | **Sand** | +крит |
| Light | Dark | **Eclipse** | +урон по всем (бешеная редкость) |
| Fire | Dark | **Inferno** | +bleed |
| Water | Light | **Prism** | +точность |
| Earth | Light | **Crystal** | +щит |

Шанс гибрида при разных стихиях = **20%** (с катализатором +35%).

---

## 3. RESONANCE_TABLE (тренировка ↔ стихия)

| Тип тренировки | Стихия с бонусом | Бонус |
|---|---|---|
| Running | Fire | +10% gains |
| Cycling | Air | +10% |
| Strength | Earth | +10% |
| Swimming | Water | +10% |
| Yoga | Light | +10% |
| Meditation | Arcane | +10% |
| HIIT | Dark | +10% |
| (другие) | — | 0% |

---

## 4. TYPE_MULTIPLIER (для Technique Card)

| Тип удара | TypeMultiplier | Особенность |
|---|---|---|
| Jab | 0.9 | быстрая, дешёвая |
| Hook | 1.0 | базовая |
| Cross | 1.1 | мощная, дорогая |
| Uppercut | 1.15 | высокий крит |
| Kick | 1.2 | высокий урон, медленная |
| Elbow | 1.1 | игнор части защиты |
| Block | 0.3 (defensive) | снижает входящий урон |

---

## 5. РЕДКОСТЬ — ДВЕ НЕЗАВИСИМЫЕ ТАБЛИЦЫ

> ⚠️ Раньше это была одна таблица с двумя столбцами, и её путали при подстановке в формулы.
> Таблицы разделены намеренно; **имена не взаимозаменяемы**.

### 5.1. `GACHA_WEIGHTS` — только для роллов гачи

Веса выпадения. Используются **исключительно** в `roll_gacha()`. Чем реже — тем меньше вес.

| Редкость | Вес |
|---|---|
| Common | 100 |
| Uncommon | 50 |
| Rare | 25 |
| Epic | 10 |
| Legendary | 3 |
| Mythic | 1 |

### 5.2. `RARITY_STAT_MULT` — только для силы

Множитель боевой силы. Используется в `effectivePower()` и в статах предметов.
Чем реже — тем **больше** множитель.

| Редкость | Множитель |
|---|---|
| Common | 1.0 |
| Uncommon | 1.15 |
| Rare | 1.35 |
| Epic | 1.6 |
| Legendary | 2.0 |
| Mythic | 2.5 |

**Никогда не подставлять `GACHA_WEIGHTS` в формулы силы:** порядок весов там обратный,
и матчмейкинг начнёт считать колоду из Common сильнее колоды из Mythic (в 40 раз).

---

## 6. GACHA DROP_TABLES

### 6.1. Стандартный баннер существ
| Редкость | Шанс |
|---|---|
| Common | 50.0% |
| Uncommon | 30.0% |
| Rare | 14.0% |
| Epic | 4.5% |
| Legendary | 1.3% |
| Mythic | 0.2% |

### 6.2. Premium-баннер (за Gems)
| Редкость | Шанс |
|---|---|
| Common | 35.0% |
| Uncommon | 32.0% |
| Rare | 22.0% |
| Epic | 8.0% |
| Legendary | 2.7% |
| Mythic | 0.3% |

### 6.3. Pity system
- `rare_pity = 10`: 10 pull'ов без Rare+ → гарантия Rare+.
- `epic_pity = 50`: 50 pull'ов без Epic+ → гарантия Epic+.
- Счётчики обнуляются после срабатывания.

---

## 7. ЭКОНОМИКА — цены

### 7.1. Расходники (Koins)
| Предмет | Цена | Эффект |
|---|---|---|
| Apple | 20 Koins | hunger +20 |
| Steak | 80 Koins | hunger +60, mood +5 |
| Energy Drink | 50 Koins | energy +40 |
| Soap | 30 Koins | hygiene +50 |
| Shampoo | 60 Koins | hygiene +80, mood +5 |
| Medicine | 100 Koins | cure weakness |
| Vitamins | 150 Koins | +20% XP на 1ч |

### 7.2. Гача
| Покупка | Цена |
|---|---|
| 1 pull | 100 Gems |
| 10 pull | 900 Gems (−10%) |
| 30 pull | 2500 Gems (−17%) |

### 7.3. Бридинг
| Предмет | Цена |
|---|---|
| Love Crystal | 200 Koins (или 1 раз/неделю бесплатно) |
| Mutation Catalyst | 50 Gems |
| Incubate Skip | 50 Gems |

### 7.4. Экипировка
| Слот | Common | Rare | Epic |
|---|---|---|---|
| Оружие | 200 K | 800 K | 200 Gems |
| Броня | 150 K | 600 K | 150 Gems |
| Аксессуар | 100 K | 400 K | 100 Gems |

---

## 8. БОЕВОЙ БАЛАНС

### 8.1. HP-формула
```
maxHP = 1000 + END_stat * 10 + gear_end_bonus * 10
// при END=100: 2000 HP, при END=200: 3000 HP
```

### 8.2. Damage-формула (детально)
```
damage = card.baseDamage
    * elementMultiplier(atk, def)
    * (1 + techCardBonus(card.type, attacker.genome.tech_affinity))   // +0 или +0.15
    * moodMultiplier(attacker.mood)
    * (1 - defender.defenseRatio)                                      // defense = FOC/1000, кап 0.5
    * rngVariance(rng)                                                 // [0.9, 1.1]

if crit (rng < critChance): damage *= 1.8
```

### 8.3. Defense ratio
```
defenseRatio = min(FOC / 1000, 0.5)
// FOC=100 → 10% сокращения, FOC=500 → 50%
```

### 8.4. Stamina в бою
```
startingStamina = 100 + END_stat / 5
each card cost = card.staminaCost
if stamina < cost: card plays with 50% damage
stamina regen per round = 5 + END_stat / 50
```

---

## 9. ПРОГРЕССИЯ — цели по неделям

| Срок после начала | Ожидаемый уровень питомца | Лига | Кол-во Technique Cards |
|---|---|---|---|
| Неделя 1 | 8 (Baby → Teen transition) | Bronze | 5 |
| Неделя 2 | 18 | Silver low | 12 |
| Неделя 4 | 30 (Teen → Adult) | Silver high | 25 |
| Неделя 8 | 45 | Gold | 50 |
| Неделя 12 | 55 (Premium branch) | Platinum | 80 |
| Неделя 24 | 60+ (cap) | Diamond+ | 150+ |

---

## 10. RETENTION И ECONOMY — sanity checks

| Метрика | Целевой диапазон |
|---|---|
| Koins заработано / потрачено за неделю | ratio 1.0–1.2 |
| Gems流入 / расход | ratio 0.8–1.1 |
| Среднее число дуэлей/день на active | 1.5–3.0 |
| Среднее число breeding/неделю | 0.5–1.0 |
| Доля игроков, дошедших до Adult | ≥ 40% (M3) |
| Доля платящих | 2–5% |

---

## 11. КОРРЕКТИРОВКИ

- Любое изменение этого файла → A/B-тест на 5% аудитории минимум 7 дней.
- Лог изменений — внизу ( changelog).

---

## 12. CHANGELOG

| Дата | Версия | Изменение |
|---|---|---|
| 2026-07-20 | v1.0 | Начальная версия баланса |

---

## 13. СВЯЗАННЫЕ ДОКУМЕНТЫ

- `docs/04-core/CORE_FORMULAS.md` — формулы.
- `docs/01-design/GDD.md` — геймдизайн.
- `docs/02-mechanics/*` — механики.
