# CLIENT: Apple Watch (watchOS)

> Нативный клиент GOCHYA для Apple Watch. **Unity не поддерживает watchOS**, поэтому клиент обязательно нативный.

---

## 1. ТЕХНОЛОГИЧЕСКИЙ СТЕК

| Компонент | Технология |
|---|---|
| Язык | Swift 5.9+ |
| UI | SwiftUI + WatchKit |
| Минимальная версия | watchOS 10.0 |
| Рендер существа | **SpriteKit** (soft 3D / 2.5D, скелетная анимация) — решение аудита P5 |
| Скелетная анимация | Spine / Rive → SpriteKit atlas |
| Анимация UI | Lottie / Native SwiftUI |
| Сенсоры | CoreMotion (`CMMotionManager`) — акселерометр и гироскоп |
| Здоровье | HealthKit (`HKHealthStore`) |
| Пульс realtime | `HKWorkoutSession` + `HKLiveWorkoutBuilder` (только так; см. §4.3 edge cases) |
| Связь с iPhone | Watch Connectivity (`WCSession`) |
| IAP | StoreKit 2 |
| Core-интеграция | `.xcframework` (iOS + watchOS slices) через Swift bridge (FFI) |
| AOD | WidgetKit complications |

> **Решение по рендеру (audit P5):** SpriteKit как основной путь.
> SceneKit soft-deprecated (WWDC 2025), RealityKit на watchOS недоступен, Metal raw слишком
> дорого для MVP. SpriteKit + параллакс + скелетная анимация (Spine/Rive) даёт именно тот
> «soft 3D / 2.5D» look, который описан в `ART_BIBLE.md`. Премиально, не ретро.
>
> Принцип `ART_BIBLE.md` §10 «модель авторится один раз» сохраняется: источник — Spine/Rive
> sketal animation, экспорт под SpriteKit (watchOS), Filament (Wear OS), Rive (companion).

---

## 2. АРХИТЕКТУРА ПРИЛОЖЕНИЯ

```
watchos/
├── GOCHYA Watch App.swift           ← @main, WatchApp
├── App/                             ← навигация, TabView/NavigationView
│   ├── RootView.swift
│   └── Router.swift
├── Features/
│   ├── Pet/                         ← главный экран питомца
│   │   ├── PetView.swift
│   │   ├── PetViewModel.swift
│   │   └── NeedsView.swift          ← 4 индикатора потребностей
│   ├── Care/                        ← кормление, чистка, сон
│   ├── Training/                    ← мини-игры тренировок
│   ├── Dojo/                        ← запись ударов (USP-1)
│   │   ├── DojoView.swift
│   │   ├── RecordingSession.swift   ← CoreMotion + HR
│   │   └── PunchClassifier.swift    ← Core ML / DTW
│   ├── PvP/                         ← быстрый матч, рейтинг
│   └── Profile/                     ← настройки, кольца активности
├── Core/                            ← bridge к Shared Core
│   ├── CoreBridge.swift             ← FFI-обёртки
│   └── Types.swift                  ← Swift-модели → Core-модели
├── Services/
│   ├── HealthKitService.swift       ← шаги, сон, тренировки
│   ├── HeartRateService.swift       ← HKWorkoutSession
│   ├── SensorService.swift          ← CoreMotion
│   ├── NetworkService.swift         ← REST/WS к серверу
│   ├── OfflineCache.swift           ← локальное сохранение
│   └── Notifications.swift          ← локальные пушы
├── Rendering/
│   ├── PetScene.swift               ← SpriteKit scene (soft 3D / 2.5D)
│   ├── ParticleSystem.swift
│   └── Shaders/
└── Resources/
    ├── Assets.xcassets              ← текстуры, модели
    ├── Animations/                  ← Spine/Lottie
    └── Models/                      ← Core ML модель классификатора
```

---

## 3. ГЛАВНЫЙ ЭКРАН (Pet View)

```
        ╭────────────────────────╮
       │                          │
      │      ┌──────────┐         │   ← SpriteKit: питомец в центре (soft 3D / 2.5D)
      │      │   PET    │         │      круглый экран, безопасные зоны
      │      │  (3D)    │         │
       │     └──────────┘        │
        ╲   ┌───┬───┬───┬───┐   ╱    ← 4 индикатора потребностей
         ╲  │ 🍔 │ 💤 │ 🛁 │ ❤️ │  ╱      компактные кольца/дуги
          ╲ └───┴───┴───┴───┘ ╱
           ╲ ──── Activity ───╱       ← мини-кольцо Vitality
            ╲  ◐ Crown: Menu  ╱       ← подсказка про Crown
```

- Вращение Digital Crown → радиальное меню (Уход/Тренировка/Бой/Профиль).
- Тап по питомцу → обнимашка (+Mood).
- Long press → быстрые действия.

---

## 4. DOJOMODE — ЗАПИСЬ УДАРА

### 4.1. Контракт
```
class RecordingSession:
    - startPreflight() → мониторит HR 8 сек, возвращает baseline + confidence
    - startRecording(duration: 6s) → пишет accel+gyro+HR
    - stop() → возвращает RecordingResult {metrics, signal_hash, heart_evidence}
```

### 4.2. Использование CoreMotion
```swift
let motion = CMMotionManager()
motion.deviceMotionUpdateInterval = 1.0 / 50.0   // 50 Гц
motion.startDeviceMotionUpdates(to: queue) { data, _ in
    // data.userAcceleration, data.rotationRate
}
```

### 4.3. Пульс realtime — HKWorkoutSession (с degraded mode)
- **Обязательно** активная `HKWorkoutSession` (иначе пульс с задержкой 5–30 сек).
- `HKLiveWorkoutBuilder` собирает HR-сэмплы.
- См. `MECHANIC_HEART_GATE.md`.

> ⚠️ **Edge cases (audit A5/B1):**
> 1. **Одновременно только одна `HKWorkoutSession`.** Если у пользователя уже запущена тренировка (бег, вело, Apple Fitness+), GOCHYA **не сможет** запустить свою. Dojo должен проверить `HKHealthStore.biologicalSex()`/`workoutSession` состояние и в случае конфликта:
>    - показать мягкое сообщение «Сначала заверши активную тренировку, чтобы записать приём»;
>    - НЕ запускать вторую session (это невозможно);
>    - предложить fallback: `/dojo/preflight` вернёт `degraded=true`, и запись пройдёт без realtime HR → `heartScore=0` (карта будет Common/Uncommon, но игрок не заблокирован).
> 2. **Не вызывать `finishWorkout()`** — иначе сессия запишется в Health как тренировка, поднимет оранжевое Activity-кольцо и разозлит пользователя. Использовать `discardWorkout()` вместо `finishWorkout()` для отмены без сохранения.
> 3. **Watch locked** — доставка HR может задерживаться при блокировке экрана. Держать экран активным во время Dojo (но dim для экономии батареи — см. `MECHANIC_HEART_GATE.md` §4).

### 4.4. Классификатор
- **MVP:** DTW (Dynamic Time Warping) по 5 шаблонам — см. `docs/02-mechanics/MECHANIC_ML_CLASSIFIER.md`.
- **Фаза 2:** Core ML модель (`PunchClassifier.mlmodel`), обученная на размеченных записях.

---

## 5. HEALTHKIT — ЧТЕНИЕ АКТИВНОСТИ

### 5.1. Запросы доступа
```swift
let readTypes: Set<HKObjectType> = [
    HKQuantityType.quantityType(forIdentifier: .stepCount)!,
    HKCategoryType.categoryType(forIdentifier: .sleepAnalysis)!,
    HKQuantityType.quantityType(forIdentifier: .activeEnergyBurned)!,
    HKQuantityType.quantityType(forIdentifier: .appleExerciseTime)!,
    HKQuantityType.quantityType(forIdentifier: .heartRate)!,
    HKWorkoutType.workoutType(),
]
// Write types (audit C7): НЕ nil — нам нужно писать калории/тренировки из Dojo
let shareTypes: Set<HKSampleType> = [
    HKQuantityType.quantityType(forIdentifier: .activeEnergyBurned)!,
    HKQuantityType.quantityType(forIdentifier: .appleExerciseTime)!,
    HKWorkoutType.workoutType(),
]
healthStore.requestAuthorization(toShare: shareTypes, read: readTypes)
```

### 5.2. Стратегия чтения
- **Не опрашивать постоянно.** Использовать `HKObserverQuery` + background delivery.
- При открытии приложения — `HKStatisticsQuery` за текущий день.
- Сон — `HKSampleQuery` за прошлую ночь.

### 5.3. Privacy
- `NSHealthShareUsageDescription` в Info.plist — обязательное человекочитаемое описание.
- Сырые данные не покидают устройство. На сервер — только агрегаты.

---

## 6. AOD (Always-On Display)

- Использовать `WidgetKit` complications для AOD-режима.
- Альтернатива — собственный AOD-режим в приложении с `isLuminanceReduced`.
- **Требования:**
  - Монохром/один акцент.
  - 1 FPS.
  - Минимум CPU/GPU.
  - Питомец — стилизованная пиктограмма, состояние (сон/бодр).

---

## 7. СВЯЗЬ С IPHONE (Companion)

- `WCSession`:
  - `transferUserInfo(_:)` — для надёжной доставки данных (магазин и т.д.).
  - `sendMessage(_:replyHandler:)` — для интерактивных запросов когда оба активны.
  - `transferFile(_:metadata:)` — для больших артефактов.
- Если iPhone недоступен — часы работают автономно (офлайн-режим, sync при reconnect).

---

## 8. STOREKIT 2 — IAP

- `Product.purchase()` async API.
- Серверная валидация через App Store Server API.
- См. `BACKEND.md` раздел IAP.

---

## 9. PERFORMANCE BUDGET (watchOS)

| Метрика | Цель |
|---|---|
| Cold start | ≤ 1.5 сек |
| Память (active) | ≤ 150 МБ |
| FPS (активная игра) | 60 (или 30 для сложных сцен) |
| FPS (AOD) | 1 |
| CPU (idle) | ≤ 5% |
| Расход батареи в фоне | ≤ 2%/час |
| Расход в Dojo (16 сек) | ≤ 0.5% |
| Размер бандла | ≤ 50 МБ |

---

## 10. CORE BRIDGE (FFI)

```swift
// CoreBridge.swift
import Foundation
import GOCHYACore       ← импорт xcframework

struct CoreContext {
    static var rng: OpaquePointer?
}

func coreQualityScore(metrics: PunchMetrics, heart: HeartRateEvidence) -> Float {
    return gochya_quality_score(metrics.toC(), heart.toC())
}

func coreSimulateCombat(match: Match, seed: UInt64) -> MatchResult {
    var cMatch = match.toC()
    let cResult = gochya_simulate_combat(&cMatch, seed)
    return MatchResult.fromC(cResult)
}
```

- Все типы — в `Types.swift` (Swift ↔ C-struct).
- Каждая формула — вызов в ядро, не пересчитывается локально.

---

## 11. ТЕСТЫ

- Unit-тесты для `CoreBridge` — на детерминизм (вход → ожидаемый выход).
- UI-тесты для главных экранов.
- Performance-тесты (XCTest `measure`) для критичных функций.

---

## 12. СВЯЗАННЫЕ ДОКУМЕНТЫ

- `ARCHITECTURE.md` — общая картина.
- `docs/04-core/CORE_SPEC.md` — контракт ядра, типы.
- `docs/02-mechanics/MECHANIC_COMBAT_RECORDING.md`, `MECHANIC_HEART_GATE.md` — Dojo и пульс.
- `docs/02-mechanics/MECHANIC_SYNERGY.md` — HealthKit-чтение.
