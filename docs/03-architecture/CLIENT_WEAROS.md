# CLIENT: Galaxy Watch / Wear OS (нативный Kotlin)

> Нативный клиент GOCHYA для Samsung Galaxy Watch (Wear OS). **Unity заменён на нативный Kotlin + Filament** по результатам аудита: Unity не имеет официальной поддержки Wear OS, раздувает APK (80–150 МБ), требует native plugin для сенсоров и не гарантирует perf-бюджет.

---

## 1. ПОЧЕМУ НАТИВ, А НЕ UNITY (решение аудита S3)

| Параметр | Unity (бывший план) | Нативный Kotlin (новый план) |
|---|---|---|
| Официальная поддержка Wear OS | ❌ нет | ✅ есть |
| Cold start | 4–6 сек | <2 сек |
| APK | 80–150 МБ | <20 МБ |
| Сенсоры (accel/gyro/HR) | нужен native plugin | нативно |
| FPS на Galaxy Watch 6 | 30 (с трудом) | **60** (доказуемо) |
| IAP Galaxy Store на часах | не сертифицирован | через companion |
| Итерация арта | редактор Unity | Filament + GLTF |

**Вывод:** для нашего контента (одно low-poly существо + партиклы + UI) Unity — overkill. Нативный Filament даёт ту же или лучшую графику (PBR-шейдинг, скелетная анимация, партиклы) при в 2–3× более быстром cold start и в 5–10× меньшем APK. **30 FPS — нижний порог, реально 60.**

---

## 2. ТЕХНОЛОГИЧЕСКИЙ СТЕК

| Компонент | Технология |
|---|---|
| Язык | Kotlin 1.9+ |
| Минимальная версия | Wear OS 3.0 (API 30) — Galaxy Watch 4+ |
| UI | Jetpack Compose for Wear OS |
| Рендер существа | **Filament** (PBR-движок Google) через `Choreographer` |
| Анимация UI | Lottie / Rive (compose-обёртки) |
| Скелетная анимация существа | Filament glTF + animation engine |
| Сенсоры | Android `SensorManager` напрямую |
| Здоровье | Health Services API (`androidx.health.services`) |
| Пульс realtime | Samsung Health Sensor SDK (требует partner approval — см. RISKS R2) |
| IAP | **через companion/телефон** (Galaxy Store IAP на часах не сертифицирован) |
| Core-интеграция | JNI напрямую к `libgochya_core.so` |
| AOD | `AmbientModeSupport` (Wear OS) |
| ML (классификатор ударов) | ONNX Runtime Android (или DTW на Kotlin — см. `MECHANIC_ML_CLASSIFIER.md`) |

---

## 3. АРХИТЕКТУРА ПРОЕКТА

```
wearos/
├── app/
│   ├── build.gradle.kts                 ← wearApp module
│   ├── src/main/
│   │   ├── AndroidManifest.xml          ← uses-feature watch
│   │   ├── kotlin/com/gochya/wearos/
│   │   │   ├── GochyaApplication.kt     ← Application, DI init
│   │   │   ├── MainActivity.kt          ← ComponentActivity, setContent
│   │   │   ├── ui/
│   │   │   │   ├── theme/               ← Material3 Wear colors, typography
│   │   │   │   ├── navigation/          ← Wear NavHost (SwipeDismissable)
│   │   │   │   └── screens/
│   │   │   │       ├── PetScreen.kt     ← главный экран (Filament surface)
│   │   │   │       ├── CareScreen.kt
│   │   │   │       ├── TrainingScreen.kt
│   │   │   │       ├── DojoScreen.kt    ← запись удара (USP-1)
│   │   │   │       ├── PvPScreen.kt
│   │   │   │       └── ProfileScreen.kt
│   │   │   ├── render/
│   │   │   │   ├── FilamentEngine.kt    ← Filament init, surface, lifecycle
│   │   │   │   ├── PetModel.kt          ← glTF загрузка, анимации
│   │   │   │   ├── ParticleSystem.kt
│   │   │   │   └── Shaders/             ← кастомные material (.mat)
│   │   │   ├── dojo/
│   │   │   │   ├── RecordingSession.kt  ← SensorManager + HR
│   │   │   │   ├── PunchClassifier.kt   ← ONNX / DTW
│   │   │   │   └── SignalProcessor.kt   ← detrend, normalise, FFT (для entropy)
│   │   │   ├── core/
│   │   │   │   ├── CoreBridge.kt        ← JNI обёртки
│   │   │   │   └── Types.kt             ← Kotlin ↔ C-structs (JNA/JNI)
│   │   │   ├── services/
│   │   │   │   ├── HealthService.kt     ← Health Services / Samsung Health
│   │   │   │   ├── HeartRateService.kt
│   │   │   │   ├── SensorService.kt
│   │   │   │   ├── NetworkService.kt    ← Ktor / Retrofit
│   │   │   │   ├── OfflineCache.kt      ← Room DB
│   │   │   │   └── Notifications.kt
│   │   │   └── util/
│   │   ├── assets/
│   │   │   ├── models/                  ← .glb / .gltf (существо, декор)
│   │   │   ├── animations/              ← .riv / .json (Lottie UI)
│   │   │   └── ml/
│   │   │       └── punch_classifier.onnx
│   │   └── res/
│   │       ├── drawable/                ← иконки, текстуры UI
│   │       └── values/
│   └── proguard-rules.pro               ← keep-правила для JNI
├── jniLibs/
│   └── arm64-v8a/
│       └── libgochya_core.so            ← Shared Core (Rust)
└── ...
```

---

## 4. FILAMENT — РЕНДЕР СУЩЕСТВА

### 4.1. Почему Filament
- PBR-движок от Google, оптимизирован под мобильные GPU (включая Mali на Exynos).
- Поддержка glTF 2.0 (скелетная анимация, morph targets).
- Низкий overhead: 60 FPS на Galaxy Watch 6 для одной low-poly модели + нескольких партиклов — доказуемо.
- Малый размер: Filament runtime ~3 МБ.
- Активная поддержка, обновления.

### 4.2. Инициализация
```kotlin
class FilamentEngine {
    private lateinit var engine: Engine
    private lateinit var renderer: Renderer
    private lateinit var scene: Scene
    private lateinit var view: View
    private lateinit var camera: Camera
    private lateinit var surfaceView: SurfaceView

    fun init(surfaceView: SurfaceView) {
        engine = Engine.create()
        renderer = engine.createRenderer()
        scene = engine.createScene()
        view = engine.createView()
        camera = engine.createCamera(engine.entityManager.create).let {
            view.camera = it; it
        }
        view.scene = scene

        // Choreographer для 60 FPS
        Choreographer.getInstance().postFrameCallback(object : FrameCallback {
            override fun doFrame(frameTimeNanos: Long) {
                if (renderer.beginFrame(swapChain!!, frameTimeNanos)) {
                    renderer.render(view)
                    renderer.endFrame()
                }
                Choreographer.getInstance().postFrameCallback(this)
            }
        })
    }
}
```

### 4.3. Загрузка существа (glTF)
```kotlin
fun loadPet(uri: Uri, animations: List<String>) {
    val assetLoader = AssetLoader(engine, MaterialProvider(engine), EntityManager.get())
    val resourceLoader = ResourceLoader(engine)
    val asset = assetLoader.createAssetFromFile(uri.path)
    resourceLoader.addResourceData(uri, readBuffer(uri))
    resourceLoader.loadResources(asset!!)
    asset.applyChanges()

    asset.entities.forEach { scene.addEntity(it) }
    animations.forEach { animName ->
        val animator = asset.instance.animator
        animator.applyAnimation(animName, 0f)
        animator.updateBoneMatrices()
    }
}
```

### 4.4. AOD (Ambient Mode)
```kotlin
class AmbientCallback : AmbientModeSupport.AmbientCallback() {
    override fun onEnterAmbient(ambientDetails: Bundle?) {
        // Упростить сцену: убрать партиклы, частицы, освещение
        scene.removeEntity(particlesEntity)
        view.blitMode = View.BlitMode.OPAQUE
        // FPS降到1
        choreographerInstance.setLowLatency(false)
    }
    override fun onExitAmbient() { /* восстановление */ }
}
```

---

## 5. DOJО — ЗАПИСЬ УДАРА

### 5.1. Сенсоры (нативно, без плагинов)
```kotlin
class RecordingSession(context: Context) {
    private val sm = context.getSystemService(Context.SENSOR_SERVICE) as SensorManager
    private val accel = sm.getDefaultSensor(Sensor.TYPE_ACCELEROMETER)!!
    private val gyro = sm.getDefaultSensor(Sensor.TYPE_GYROSCOPE)!!

    fun startRecording(durationSec: Float, onResult: (RecordingResult) -> Unit) {
        val samples = mutableListOf<FloatArray>()
        sm.registerListener(object : SensorEventListener {
            override fun onSensorChanged(event: SensorEvent) {
                samples.add(floatArrayOf(
                    event.values[0], event.values[1], event.values[2],  // accel
                    event.timestamp / 1e9f
                ))
            }
            override fun onAccuracyChanged(p0: Sensor?, p1: Int) {}
        }, accel, SensorManager.SENSOR_DELAY_GAME)  // ~50 Гц

        Handler(Looper.getMainLooper()).postDelayed({
            sm.unregisterListener(/* ... */)
            onResult(process(samples))
        }, (durationSec * 1000).toLong())
    }
}
```

### 5.2. Пульс realtime — Samsung Health Sensor SDK
- Требует partner approval от Samsung (см. RISKS R2).
- Если partner status не получен → **degraded mode**: `Sensor.TYPE_HEART_RATE` (задержка 5–15 сек, но не блокирует Dojo полностью).
- См. `MECHANIC_HEART_GATE.md` §5 для контракта.

### 5.3. Классификатор
- MVP: DTW на Kotlin (5 шаблонов) — см. `MECHANIC_ML_CLASSIFIER.md`.
- Фаза 2: ONNX Runtime Android (`punch_classifier.onnx`).

---

## 6. HEALTH SERVICES API — ЧТЕНИЕ АКТИВНОСТИ

### 6.1. Шаги
```kotlin
val client = HealthServices.getClient(context)
val passiveMonitorClient = client.passiveMonitoringClient

// регистрация на шаги (фоновой мониторинг системой, не игрой)
val stepsGoal = PassiveGoal {
    setDataType(DataType.STEPS_DAILY)
    // ...
}
passiveMonitorClient.passiveGoalsCallback = ...
```

### 6.2. Sleep (через Samsung Health SDK или Health Services `SleepStage`)
```kotlin
val exerciseClient = client.exerciseClient
// Sleep stages: UNKNOWN, AWAKE, LIGHT, DEEP, REM
```

### 6.3. Permissions
```xml
<uses-permission android:name="android.permission.ACTIVITY_RECOGNITION" />
<uses-permission android:name="android.permission.BODY_SENSORS" />
<!-- опционально для местоположения тренировок -->
<uses-permission android:name="android.permission.ACCESS_FINE_LOCATION" />
```

---

## 7. IAP — ЧЕРЕЗ COMPANION

**Galaxy Store IAP на часах не сертифицирован Samsung.** Все покупки выполняются на companion-телефоне:

1. Игрок открывает магазин на часах → перенаправляется на телефон (deep link).
2. Companion выполняет покупку (Galaxy Store IAP / Google Play Billing).
3. Companion синхронизирует с сервером → сервер обновляет инвентарь.
4. Часы получают обновлённый инвентарь при следующей синхронизации.

Альтернатива для Wear OS на Google Play (Pixel Watch и др.): **Google Play Billing on Wear OS** (доступно с Wear OS 3, `com.android.billing`). Но Galaxy Watch первичны → используем companion-flow как основной.

---

## 8. BEZEL (ROTATING BEZEL)

- `PhysicalRotationSensor` (Wear OS) или `RotaryEncoder` через Compose.
- Используется для радиального меню (Уход/Тренировка/Бой/Профиль).

```kotlin
val rotaryInput = rememberRotaryInput()
LaunchedEffect(rotaryInput) {
    rotaryInput.rotaryScrollCollector(initial = 0f) { scroll, _ ->
        // обновить активный элемент радиального меню
    }
}
```

---

## 9. NETWORKING

- **Ktor Client** или **Retrofit** для REST.
- WebSocket: **Ktor WebSocket** для живых турниров.
- Offline-кэш: **Room** (SQLite).

---

## 10. PERFORMANCE BUDGET (Wear OS / натив)

| Метрика | Цель | Реалистичность |
|---|---|---|
| Cold start | ≤ 2 сек | ✅ достижимо (нативный запуск, <2 сек типично) |
| Память (active) | ≤ 150 МБ | ✅ (Filament ~10 МБ + модели) |
| FPS (активная игра) | **60** | ✅ (Filament на одной модели) |
| FPS (AOD) | 1 | ✅ (Ambient mode) |
| CPU (idle) | ≤ 5% | ✅ (без фоновых опросов) |
| Расход батареи в фоне | ≤ 2%/час | ✅ |
| Расход в Dojo (16 сек) | ≤ 0.5% | ⚠️ требует dim-экрана во время записи |
| Размер APK | ≤ 20 МБ | ✅ (нативный стек) |

---

## 11. CORE BRIDGE (JNI)

```kotlin
object CoreBridge {
    init {
        System.loadLibrary("gochya_core")
    }

    external fun gochya_quality_score(
        metrics: PunchMetricsC,
        heart: HeartRateEvidenceC
    ): Int  // 0..100

    external fun gochya_simulate_combat(
        match: MatchC,
        seed: Long,
        outResult: ByteBuffer  // out-указатель для большой структуры
    ): Int  // CoreError
}
```

> ⚠️ **Важно:** большие структуры (например `MatchResult` ~400 байт) передаются через out-параметр (`ByteBuffer`), а не возвратом by value — это исправление FFI-ошибки аудита (T4).

- Marshalling Kotlin ↔ C-structs — в `Types.kt`.
- Все формулы — вызовы в ядро, не пересчитываются локально.

---

## 12. ANDROIDMANIFEST

```xml
<manifest xmlns:android="http://schemas.android.com/apk/res/android">
    <uses-feature android:name="android.hardware.type.watch" />
    <uses-feature android:name="android.hardware.sensor.accelerometer" required="true" />
    <uses-feature android:name="android.hardware.sensor.gyroscope" required="true" />
    <uses-feature android:name="android.hardware.sensor.heart_rate" required="false" />

    <uses-permission android:name="android.permission.WAKE_LOCK" />
    <uses-permission android:name="android.permission.ACTIVITY_RECOGNITION" />
    <uses-permission android:name="android.permission.BODY_SENSORS" />
    <uses-permission android:name="android.permission.INTERNET" />

    <application
        android:name=".GochyaApplication"
        android:label="GOCHYA"
        android:icon="@mipmap/ic_launcher"
        android:supportsRtl="true"
        android:theme="@android:style/Theme.DeviceDefault">

        <meta-data android:name="com.google.android.wearable.standalone"
                   android:value="false" />

        <activity android:name=".MainActivity">
            <intent-filter>
                <action android:name="android.intent.action.MAIN" />
                <category android:name="android.intent.category.LAUNCHER" />
            </intent-filter>
        </activity>
    </application>
</manifest>
```

- `minSdkVersion = 30` (Wear OS 3.0).
- `targetSdkVersion = latest`.
- `standalone="false"` — требует companion для IAP и сложных экранов.

---

## 13. ТЕСТЫ

- Unit-тесты для `CoreBridge` (детерминизм JNI).
- Compose UI-тесты для главных экранов.
- Espresso для AOD-переходов.
- Filament: визуальные regression-тесты (golden images).

---

## 14. СВЯЗАННЫЕ ДОКУМЕНТЫ

- `ARCHITECTURE.md` — общая картина.
- `docs/04-core/CORE_SPEC.md` — контракт ядра.
- `docs/02-mechanics/MECHANIC_COMBAT_RECORDING.md` — Dojo.
- `docs/02-mechanics/MECHANIC_HEART_GATE.md` — пульс-валидация.
- `docs/02-mechanics/MECHANIC_ML_CLASSIFIER.md` — классификатор ударов.
- `docs/02-mechanics/MECHANIC_SYNERGY.md` — Health Services чтение.
- `docs/07-roadmap/RISKS.md` R2 — Samsung Health partner approval (single point of failure).
