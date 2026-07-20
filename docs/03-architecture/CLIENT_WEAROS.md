# CLIENT: Galaxy Watch / Wear OS (Unity)

> Unity-клиент GOCHYA для Samsung Galaxy Watch (Wear OS). Выбран Unity по требованию заказчика для лучшей графики и анимации.

---

## 1. ТЕХНОЛОГИЧЕСКИЙ СТЕК

| Компонент | Технология |
|---|---|
| Движок | Unity LTS (2022.3+ или Unity 6) |
| Язык | C# |
| Минимальная версия | Wear OS 3.0 (API 30) — Galaxy Watch 4+ |
| Рендер | URP (Universal Render Pipeline) для оптимизации |
| Сенсоры | Android `SensorManager` |
| Здоровье | Samsung Health SDK + Health Services API |
| Пульс realtime | Samsung Health Sensors API (`SensorType.HEART_RATE`) |
| IAP | Galaxy Store IAP + Google Play Billing |
| Core-интеграция | Native Android plugin (.so/.aar) + P/Invoke |
| AOD | Unity Ambient Mode (`WearableManager` / `AmbientMode`) |
| ML | TensorFlow Lite Unity (или ONNX Runtime) |

---

## 2. АРХИТЕКТУРА UNITY-ПРОЕКТА

```
wearos/
├── Assets/
│   ├── _Project/
│   │   ├── App/                        ← bootstrap, scenes
│   │   │   ├── MainScene.unity
│   │   │   └── Bootstrapper.cs
│   │   ├── Features/
│   │   │   ├── Pet/                    ← PetView, PetController
│   │   │   ├── Care/
│   │   │   ├── Training/
│   │   │   ├── Dojo/                   ← запись ударов (USP-1)
│   │   │   │   ├── RecordingSession.cs
│   │   │   │   ├── AndroidSensorPlugin.cs
│   │   │   │   └── PunchClassifier.cs
│   │   │   ├── PvP/
│   │   │   └── Profile/
│   │   ├── Core/                       ← bridge к Shared Core
│   │   │   ├── CoreBindings.cs         ← P/Invoke
│   │   │   └── Types.cs
│   │   ├── Services/
│   │   │   ├── HealthService.cs        ← Samsung Health / Health Services
│   │   │   ├── HeartRateService.cs
│   │   │   ├── SensorService.cs
│   │   │   ├── NetworkService.cs
│   │   │   ├── OfflineCache.cs
│   │   │   └── Notifications.cs
│   │   ├── Rendering/
│   │   │   ├── PetRenderer.cs          ← шейдеры, ParticleSystem
│   │   │   └── Shaders/
│   │   ├── UI/                         ← UI Toolkit или uGUI
│   │   └── Utils/
│   ├── Plugins/
│   │   └── Android/
│   │       ├── gochya-core.aar         ← native bridge к Shared Core
│   │       └── GOCHYACore.jar
│   └── TLSSettings/                    ← если есть networking
├── Packages/
│   └── manifest.json                   ← TFLite, URP, Samsung SDK
└── ...
```

---

## 3. URP НАСТРОЙКИ ДЛЯ WEAR OS

- Renderer: **UniversalRenderer** с MSAA off (или 2x).
- Shadow maps: 512, только для близких объектов.
- HDR: **off** (экономия батареи).
- Texture compression: **ASTC** (6×6 для UI, 4×4 для существа).
- Frame pacing: **30 FPS** целевой, динамический (Application.targetFrameRate).
- VSync: off, вручную через `QualitySettings`.

---

## 4. DOJOMODE — ЗАПИСЬ УДАРА

### 4.1. Android Sensor Plugin
```csharp
// P/Invoke к нативному плагину
[DllImport("gochya-native")]
private static extern IntPtr start_recording(float durationSec);

[DllImport("gochya-native")]
private static extern RecordingResultNative stop_recording();
```

Нативная часть (Kotlin/C++):
```kotlin
val sensorManager = context.getSystemService(SENSOR_SERVICE) as SensorManager
val accel = sensorManager.getDefaultSensor(Sensor.TYPE_ACCELEROMETER)
val gyro = sensorManager.getDefaultSensor(Sensor.TYPE_GYROSCOPE)
sensorManager.registerListener(this, accel, SensorManager.SENSOR_DELAY_GAME) // ~50 Гц
```

### 4.2. Пульс realtime
- Samsung Health Sensors API (`com.samsung.android.sdk.health.sensors`).
- Требуется регистрация приложения и permission `BODY_SENSORS`.
- См. `MECHANIC_HEART_GATE.md`.

### 4.3. Классификатор
- **MVP:** DTW по шаблонам (C# реализация).
- **Фаза 2:** TensorFlow Lite модель (`punch_classifier.tflite`).

---

## 5. SAMSUNG HEALTH / HEALTH SERVICES

### 5.1. Чтение шагов
```kotlin
// Health Services
val client = HealthServices.getClient(context)
val sensorClient = client.sensorClient
val steps = DataType(STEPS_TOTAL, AggregateDataType.DELTA)
sensorClient.registerListener(listener, setOf(steps))
```

### 5.2. Sleep (Samsung Health SDK)
- `SamsungHealthDataReader.readData().setDataType(SleepDataType)`
- Возвращает staged sleep (light/deep/rem/awake).

### 5.3. Тренировки
- `ExerciseClient` для tracking (бег, вело и т.д.).

### 5.4. Permissions
- `ACTIVITY_RECOGNITION`, `BODY_SENSORS`, `ACCESS_FINE_LOCATION` (опц.).

---

## 6. AOD (Ambient Mode)

- Использовать Samsung Accessory SDK + `AmbientMode` (Wearable).
- При входе в ambient:
  - `Application.targetFrameRate = 1`
  - Снизить яркость, упростить шейдер
  - Отключить частицы
- При выходе — восстановить 30 FPS.

```csharp
void OnAmbientUpdate() {
    // обновить минималистичную сцену
    // монохромный питомец, 1 кольцо активности
}
```

---

## 7. BEZEL (ROTATING BEZEL)

- `WearableActionDrawerView` или собственный обработчик `RotateGesture`.
- Использовать для радиального меню (Уход/Тренировка/Бой/Профиль).

---

## 8. NETWORKING

- `UnityWebRequest` для REST.
- Native WebSocket для живых турниров.
- Offline-кэш в `Application.persistentDataPath`.

---

## 9. PERFORMANCE BUDGET (Wear OS / Unity)

| Метрика | Цель |
|---|---|
| Cold start | ≤ 3 сек |
| Память (active) | ≤ 250 МБ |
| FPS (активная игра) | 30 |
| FPS (AOD) | 1 |
| CPU (idle) | ≤ 8% |
| Расход батареи в фоне | ≤ 2%/час |
| Расход в Dojo (16 сек) | ≤ 0.5% |
| Размер APK | ≤ 50 МБ |

---

## 10. CORE BRIDGE (P/INVOKE)

```csharp
// CoreBindings.cs
using System.Runtime.InteropServices;

public static class Core {
    [DllImport("gochya-core", CallingConvention = CallingConvention.Cdecl)]
    public static extern float gochya_quality_score(ref PunchMetricsC metrics, ref HeartRateEvidenceC heart);

    [DllImport("gochya-core", CallingConvention = CallingConvention.Cdecl)]
    public static extern MatchResultC gochya_simulate_combat(ref MatchC match, ulong seed);
}
```

- Marshalling C# ↔ C-structs — в `Types.cs`.
- Никаких пересчётов формул в C# — только вызовы в ядро.

---

## 11. APK ОПТИМИЗАЦИЯ

- Asset bundles — lazy loading.
- Texture compression ASTC.
- Audio — Vorbis с низким bitrate (или отключить на часах).
- IL2CPP + stripping (managed bytecode).
- Разделять ассеты «MVP» и «фаза 2» — Deliver через asset bundles.

---

## 12. СБОРКА

- Build target: **Wear OS** (Android с `wear` feature flag).
- AndroidManifest:
  - `uses-feature android:name="android.hardware.type.watch"`
  - `uses-feature android:name="android.hardware.sensor.accelerometer"`
  - `uses-feature android:name="android.hardware.sensor.heart_rate"`
- `minSdkVersion = 30`, `targetSdkVersion = latest`.
- Запуск на эмуляторе Wear OS и реальных Galaxy Watch 6/7.

---

## 13. ТЕСТЫ

- Unity Test Framework для `CoreBindings` (детерминизм).
- Edit-mode тесты для формул-обёрток.
- Play-mode тесты для UI-флоу.

---

## 14. СВЯЗАННЫЕ ДОКУМЕНТЫ

- `ARCHITECTURE.md` — общая картина.
- `docs/04-core/CORE_SPEC.md` — контракт ядра.
- `docs/02-mechanics/MECHANIC_COMBAT_RECORDING.md`, `MECHANIC_HEART_GATE.md`.
- `docs/02-mechanics/MECHANIC_SYNERGY.md` — Samsung Health чтение.
