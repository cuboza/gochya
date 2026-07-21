# План полного исправления документации GOCHYA

Полный проход по всем ~60 находкам аудита: архитектурные решения → синхронизация спеки → дополнения → платформенная переработка → косметика. Пользователь выбрал «всё сразу», MVP-план и сроки оставить как есть.

---

## Фаза A. Зафиксировать архитектурные решения (5 файлов)

### A1. Wear OS: Unity → натив (Kotlin + Compose + Filament) — `CLIENT_WEAROS.md` полная переработка
- Удалить Unity-стек, заменить на: Kotlin, Jetpack Compose for Wear OS, Filament (PBR-рендер Google) для существа, Rive/Lottie для UI-анимаций.
- Обосновать performance: 60 FPS достижимо на Galaxy Watch 6 для нашего контента (одно low-poly существо + партиклы).
- Сенсоры: нативный `SensorManager` напрямую (без Unity native plugin).
- Samsung Health Sensor SDK: оставить как зависимость, но вынести partner approval в топ-риски.
- IAP: на часах — НЕ сертифицирован Galaxy Store; покупки идут через companion/телефон. Явно разделить.
- Core-интеграция: JNI напрямую к `.so` (без слоя Unity P/Invoke).
- APK: <20 МБ (vs 50 МБ ранее).

### A2. Privacy-first финализация — `ANTICHEAT.md` переписать §3
- **S1+S2 решение:** тип удара = доверенный клиентский ввод с вероятностным аудитом; античит = heart gate + replay-detection + клиентская энтропия.
- Убрать §3.3 «спектральная подпись на сервере», §3.4 «гироскопическая сигнатура на сервере» (физически невозможны без передачи сигнала).
- Убрать §5.5 «HealthKit/Samsung Health предоставляют подписанные данные» (ложь — это metadata, подделываемая на jailbreak).
- Переписать anticheat-score: только heart gate (60) + replay hash (15) + клиентская энтропия с проверкой диапазона (15) + rate limit (10).
- Добавить §3.X «Вероятностный аудит типа удара»: сервер хранит распределение типов по игроку, аномалия (все Hook) → флаг review.

### A3. Серверная интеграция ядра — `BACKEND.md`, `ARCHITECTURE.md`, `MASTER-PROMPT.md`
- **S4 решение:** cgo (нативная статическая линковка Rust staticlib) для production. Обосновать: проще отладка, трассировка работает.
- Убрать упоминания WASM как основного пути; оставить как опцию для sandboxing (с пометкой «не для MVP»).
- Добавить про wazero (pure-Go WASM) как альтернативу, если cgo overhead станет проблемой.

### A4. watchOS рендер — SpriteKit — `CLIENT_WATCHOS.md`, `ART_BIBLE.md`
- **P5 решение:** SpriteKit (2D skeletal) как основной путь. SceneKit deprecated, RealityKit недоступен на watchOS, Metal raw слишком дорого для MVP.
- Обосновать: ART_BIBLE описывает «soft 3D / 2.5D» — SpriteKit + параллакс + скелетная анимация (Spine/Rive) даёт именно этот look. Премиально, не ретро.
- Обновить арт-пайплайн: Blender → Rive/Spine → SpriteKit atlas (вместо SceneKit .scn).

### A5. HKWorkoutSession degraded-mode — `CLIENT_WATCHOS.md`, `MECHANIC_HEART_GATE.md`, `RISKS.md`
- Добавить §Dojo edge cases: если активная тренировка уже запущена → Dojo недоступен (мягкое сообщение «сначала заверши тренировку»).
- Обязательно **не вызывать** `finishWorkout()` (иначе запишется в Health).
- Альтернатива: `HKHeartRateQuery` polling (задержка, но без workout session) — для fallback.

---

## Фаза B. Синхронизация спеки: формулы, типы, числа (8 файлов)

### B1. `qualityScore` множитель 100 — критично
- `MECHANIC_HEART_GATE.md §7`, `MECHANIC_COMBAT_RECORDING.md §7`: добавить множитель `100 *` везде, привести к диапазону 0..100.
- Зафиксировать CORE_FORMULAS как единственный источник.

### B2. `TechniqueCard` унификация
- `MECHANIC_COMBAT_RECORDING.md`: `signature: bool` → убрать (это свойство Loadout, не карты); `effect: Option<Effect>` → `effect: Effect` (плоская структура, FFI-safe); `ownerId: PlayerId` → `owner_id: [u8;16]`.

### B3. HP-формула
- `CORE_FORMULAS.md §5.2`: добавить `+ gear_end_bonus * 10` (синхронизировать с BALANCE.md §8.1).
- Переименовать `defense_from_stats` → `defenseRatio` (единое имя).

### B4. ELO «±25» убрать
- `GDD.md §5.3`: убрать «±25 за бой», оставить ссылку на формулу K=32/24 в CORE_FORMULAS.

### B5. Battle Pass уровни
- `GDD.md §6.4`: 50 → 30 уровней (синхронизировать с MVP.md).

### B6. MVP-стихии: зафиксировать 3
- `GDD.md content scope`, `MASTER-PROMPT.md`: явно «3 стартовых стихии в MVP (Fire/Water/Earth), Air добавляется в фазе 2».
- `BALANCE.md HYBRID_TABLE`: пометить гибриды с Air как «фаза 2».
- `ART_BIBLE.md`: оставить цвет Air (заранее), но пометить «фаза 2».

### B7. MVP-скоуп в MASTER-PROMPT
- `MASTER-PROMPT.md §5`: переписать Stage 1 под актуальный MVP.md (3 существа, бридинг базовый, сезоны, BP).

### B8. Бридинг правки
- `MECHANIC_BREEDING.md`: «+X%» → «+10%» (mutation catalyst); «~20+ генов → 2²⁰» → «~14 генов → тысячи комбинаций» (реалистично по CORE_SPEC).

### B9. Симбиоз правки
- `MECHANIC_SYNERGY.md`: «сон 7.5 ч» → «адаптивная цель: MA14 × 1.10, clamp 6–9»; добавить RESONANCE_TABLE (ссылка на BALANCE §3).

### B10. Породы мутаций
- `CORE_FORMULAS.md §3.4`: переименовать комментарий «поколения» → «общие предки» для `inbreedingCoeff`.

### B11. Стадии эволюции
- `MVP.md`: уточнить «3 перехода, 4 стадии (Egg/Baby/Teen/Adult), Premium в фазе 2».

---

## Фаза C. Дополнения спецификации (4 файла)

### C1. Боевой AI выбора карт — `CORE_FORMULAS.md` новый §5.2-extended + `CORE_SPEC.md` сигнатуры
- Алгоритм: жадная эвристика по ожидаемому урону с учётом stamina и текущих HP противника.
- Signature-карта: кулдаун K раундов (зафиксировать K=5 для MVP).
- Persist статус-эффектов между раундами: `ActiveEffects { stun_rounds: u8, bleed_stacks: u8 }` — добавить в `MatchResult` или внутреннее состояние симуляции.
- Stamina поле в `Match`/состоянии боя.
- AI не использует RNG для выбора (детерминизм), только для урона.

### C2. Core API дополнения — `CORE_SPEC.md §4`
- Добавить недостающие сигнатуры: `overlevelPenalty`, `comboScore`, `normPower`, `heartGate`, `heartScore`, `defenseRatio`, `startingStamina`, `staminaRegen`, `rngVariance`, `applyXp`, `applyDecaySince`, `isHybrid`.
- Расширить `apply_care_action(pet, action, item_id)` для еды/расходников.
- Добавить типы `Player`, `Wallet`, `Inventory` как core-типы (не только SQL).
- Добавить `Bracket` тип для турниров.

### C3. ML спецификация — новый файл `docs/02-mechanics/MECHANIC_ML_CLASSIFIER.md`
- MVP: DTW по 5 шаблонам.
- Сигнал: 3-axis accel magnitude + gyro magnitude (2 ряда).
- Препроцессинг: detrend, normalise, window 6 сек.
- DTW: Sakoe-Chiba band, Euclidean distance.
- precision = `1 - normalised_dtw_distance` (clamp 0..1).
- Сегментация комбо: peak detection (accel > threshold) → интервалы → `combo_len`, `rhythm_score`.
- План сбора датасета: 5 бойцов × 5 типов × 50 повторов = 1250 записей, ручная разметка.
- Паритет Core ML ↔ TFLite: единый ONNX-формат как источник, экспорт под платформы.

### C4. Античит алгоритмы (упрощённые) — `ANTICHEAT.md` переписать §3
- Replay-detection: SHA-256 от метрик + signal_summary (не raw). Nonce серверный (выдаётся в pre-flight, одноразовый).
- Клиентская энтропия: Shannon entropy на accel magnitude (формула + порог `>= 2.5 bits`).
- Вероятностный аудит типа: сервер считает распределение типов за последние 30 записей, аномалия (>80% один тип) → review.

---

## Фаза D. Технические фиксы FFI (4 файла)

### D1. Rust targets — `CORE_SPEC.md §2`, `ARCHITECTURE.md`
- Заменить «`aarch64-apple-ios` (watchOS/iOS)» на **5 таргетов**:
  - `aarch64-apple-ios` (iOS device)
  - `aarch64-apple-watchos` (watchOS device, Series 4+)
  - `aarch64-linux-android` (Android/Wear OS)
  - `x86_64-unknown-linux-gnu` (server)
  - `x86_64-apple-ios-simulator` (iOS sim)
- Убрать `wasm32-unknown-unknown` (заменить на `wasm32-wasip1` с пометкой «опц., не для MVP»).

### D2. iOS DynamicLibrary — `CLIENT_COMPANION.md §6`
- Заменить `DynamicLibrary.open('GOCHYACore.framework/...')` на `DynamicLibrary.process()` для iOS (статическая линковка).
- Обновить инструкции по сборке: Rust staticlib → CocoaPods/SPM plugin → Flutter platform channel.

### D3. C# P/Invoke — `CLIENT_WEAROS.md` (после A1 уже неактуально, но если оставляем примеры)
- Убрать возврат большой struct by value; использовать out-указатель (`gochya_simulate_combat(match, seed, out_ptr)`).
- Добавить `link.xml` для Unity IL2CPP stripping (если Unity где-то остаётся).

### D4. FFI panic safety — `CORE_SPEC.md §8`
- Добавить `std::panic::catch_unwind` во все `extern "C"` обёртки.
- Обновить пример `gochya_quality_score` с catch_unwind.
- `set_hook` для логирования паник.

### D5. Backend дополнения — `BACKEND.md`
- Добавить таблицу `refresh_tokens` (для refresh rotation).
- Добавить колонку `source_signature` в `daily_activity` (убедиться, что описано как формируется).
- Webhook верификация: JWT verify для App Store Server Notifications V2, Pub/Sub message auth для RTDN.
- Error response body структура + каталог кодов.
- API versioning: `/v1/` prefix.
- Idempotency-Key header для `/feed`, `/breed`, `/equip`.

---

## Фаза E. Платформенные правки (3 файла)

### E1. `CLIENT_WATCHOS.md`
- Рендер: SpriteKit как основной (A4).
- HKWorkoutSession degraded mode (A5).
- `requestAuthorization(toShare:)` для записи тренировок (calories) — не `nil` (C7 ANTICHEAT).

### E2. `CLIENT_WEAROS.md` — после A1 переписать полностью
- Нативный стек, сенсоры, IAP через companion, Core через JNI.

### E3. `ARCHITECTURE.md`
- 5 Rust targets (D1).
- Серверная интеграция: cgo финально (A3).
- Добавить память в performance budget.
- Убрать «Genom» опечатку → «Genome».

---

## Фаза F. Косметика и мелкие пробелы (6 файлов)

### F1. `BALANCE.md`
- Синхронизировать цены (BREED_COST_KOINS vs Love Crystal — разделить «операция» и «предмет»).
- Добавить `PULL_COST_GEMS_30x = 2500`.
- Добавить модификатор «+35% с катализатором» для гибридов.
- Пометить Air-гибриды как «фаза 2».

### F2. `ART_BIBLE.md`
- Добавить цвета для 6 отсутствующих гибридов (Mud, Smoke, Sand, Eclipse, Inferno, Prism, Crystal).
- Обновить пайплайн: SpriteKit/Rive для watchOS, Filament/GLTF для Wear OS, Rive для companion.

### F3. `UX_UI.md`
- Выровнять onboarding: 7 шагов (GDD) vs 14 (UX_UI) — выбрать 7 с подшагами.
- Вставить age-gate шаг для COPPA.
- Добавить empty/error/loading states.
- Push templates: полный каталог.

### F4. `RISKS.md`
- Поднять Samsung Health partner approval в top-5 (single point of failure для Wear OS).
- Добавить HKWorkoutSession конфликт с активными тренировками.
- Отметить Unity→натив как решённое (после A1).
- Добавить device fingerprint как tracking data (ATT/GDPR).

### F5. `MVP.md`
- Sprint 0 добавить gate: Wear OS натив спайк (валидация Filament + сенсоры на Galaxy Watch 6).
- Sprint 0 добавить gate: watchOS SpriteKit прототип.
- Выровнять скоуп под актуальный (B6, B7, B11).

### F6. `README.md`
- Обновить ссылки на новый файл `MECHANIC_ML_CLASSIFIER.md`.

---

## Порядок выполнения

1. **Фаза A** (архитектура) — 5 файлов, фиксирует решения для всех остальных.
2. **Фаза B+C** (спека) — 12 файлов, самый большой объём.
3. **Фаза D** (FFI) — 4 файла, техническая точность.
4. **Фаза E** (платформы) — 3 файла, зависит от A.
5. **Фаза F** (косметика) — 6 файлов.

Всего затронем **~20 файлов** из 23 (плюс 1 новый `MECHANIC_ML_CLASSIFIER.md`).

## Проверка после выполнения
- `find docs -name "*.md" | wc -l` — должно быть 24 (добавился ML).
- Повторный аудит консистентности формул quality_score / heart gate / HP / ELO между ядром и механиками.
- Проверка, что ни один документ не ссылается на «серверную спектральную валидацию» или «Unity на Wear OS».