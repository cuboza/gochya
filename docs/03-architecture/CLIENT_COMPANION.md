# CLIENT: Companion-приложение на телефон (Flutter)

> **Обязательное** приложение для iOS и Android. Здесь живут все «тяжёлые» экраны: магазин, гача, инвентарь, турниры, родословная, рынок.

---

## 1. ТЕХНОЛОГИЧЕСКИЙ СТЕК

| Компонент | Технология |
|---|---|
| Фреймворк | Flutter 3.16+ (stable) |
| Язык | Dart 3+ |
| State management | Riverpod (рекомендуется) или Bloc |
| Routing | go_router |
| Сеть | dio + retrofit |
| Realtime | web_socket_channel |
| Локальная БД | Drift (SQLite) или Isar |
| Health (iOS) | healthKit через `health` package |
| Health (Android) | Google Fit / Samsung Health Plugin |
| IAP (iOS) | StoreKit 2 via `in_app_purchase` |
| IAP (Android) | Google Play Billing + Galaxy Store IAP |
| Core-интеграция | FFI (`dart:ffi`) к Shared Core |

---

## 2. ПОЧЕМУ FLUTTER

- Один кодбейз под iOS и Android.
- Shared Core через `dart:ffi` — те же формулы, что на часах.
- Богатая UI-библиотека для магазина/родословной/графиков.
- Быстрый iteration cycle.
- Хорошая производительность (60 FPS на современных телефонах не проблема).

---

## 3. АРХИТЕКТУРА

```
companion/
├── lib/
│   ├── main.dart
│   ├── app/
│   │   ├── app.dart
│   │   ├── router.dart                ← go_router config
│   │   └── theme.dart
│   ├── features/
│   │   ├── home/                      ← дашборд, питомец
│   │   ├── pet/                       ← детальный профиль питомца
│   │   ├── shop/                      ← магазин, разделы
│   │   ├── gacha/                     ← гача-баннеры
│   │   ├── inventory/                 ← инвентарь, экипировка
│   │   ├── breeding/                  ← бридинг, инкубатор
│   │   ├── lineage/                   ← родословная (дерево)
│   │   ├── pvp/                       ← дуэли, лоадаут
│   │   ├── tournaments/               ← чемпионаты, бракеты
│   │   ├── leaderboard/               ← рейтинги, лиги
│   │   ├── market/                    ← P2P рынок (фаза 2)
│   │   ├── battle_pass/               ← сезонный пропуск
│   │   ├── friends/                   ← друзья, спарринг
│   │   ├── activity/                  ← кольца, журналы, графики
│   │   └── settings/
│   ├── core/                          ← FFI bridge к Shared Core
│   │   ├── core_bindings.dart         ← dart:ffi
│   │   ├── types.dart                 ← Dart ↔ C-structs
│   │   └── core_initializer.dart
│   ├── services/
│   │   ├── api_client.dart            ← dio + retrofit
│   │   ├── auth_service.dart
│   │   ├── health_service.dart        ← HealthKit / Google Fit
│   │   ├── sync_service.dart          ← часы ↔ телефон ↔ сервер
│   │   ├── watch_connection.dart      ← WatchConnectivity / WearableAPI
│   │   ├── iap_service.dart
│   │   ├── notification_service.dart
│   │   └── analytics.dart
│   ├── models/                        ← доменные модели (не DB)
│   ├── widgets/                       ← переиспользуемые UI-компоненты
│   └── utils/
├── ios/                               ← Xcode workspace
├── android/                           ← Gradle project
├── test/
└── assets/
    ├── images/
    ├── animations/                    ← Lottie / Rive
    └── core/
        └── libgochya_core.so / .dylib  ← Shared Core бинарь
```

---

## 4. ГЛАВНЫЕ ЭКРАНЫ

### 4.1. Главная (Home)
- Питомец крупно (HD-рендер, не как на часах).
- 4 индикатора потребностей.
- Кольца активности (Vitality / Rest / Focus) с прогрессом.
- Быстрые кнопки: Покормить, Тренировать, Бой.
- Бейджи: дневные задания, ивенты.

### 4.2. Магазин (Shop)
- Категории: Расходники, Косметика, Снаряжение, Декор, Яйца/Катализаторы, Спецпредложения.
- Каждый предмет: иконка, редкость, цена в Koins/Gems/Crowns.
- Гача-баннеры — отдельная вкладка с опубликованными дропами.

### 4.3. Гача (Gacha)
- Анимация pull'а (Lottie/Rive).
- Отображение редкости результата.
- Pity-индикатор (сколько pull'ов до гарантии).
- История pull'ов.

### 4.4. Инвентарь (Inventory)
- Фильтры по типу и редкости.
- Снаряжение: equip/unequip с превью effect-power.
- Technique Cards: сортировка, добавление в лоадаут.

### 4.5. Бридинг (Breeding)
- Выбор двух питомцев (свои/друга/рынок).
- Предпросмотр вероятных признаков потомства (без точных генов — иначе чит).
- Катализаторы и Love Crystal.
- Инкубатор: активные яйца с прогрессом времени.

### 4.6. Родословная (Lineage)
- Визуализация дерева предков (Flutter `CustomPainter` или `graphview` package).
- Подсветка унаследованных признаков.
- Мутации отмечены спец-иконками.

### 4.7. PvP / Турниры
- Список режимов.
- Лоадаут-конструктор.
- Сезонные чемпионаты с бракетами (`bracket` дерево).
- История боёв с replay анимацией.

### 4.8. Лидерборд
- Глобальный / Друзья / Лига.
- Топ-N сезона.
- Награды по финальному рангу.

### 4.9. Активность (Activity)
- Недельные графики: твоя активность ↔ рост питомца.
- Журнал дня с breakdownом (шаги, сон, тренировки, эффекты).
- Достижения.

---

## 5. СВЯЗЬ С ЧАСАМИ

### iOS (WatchConnectivity)
- `WCSession` на стороне телефона.
- `transferUserInfo` для надёжной доставки.
- `sendMessage` для интерактива.

### Android (Wearable API)
- `Wearable.getMessageClient` / `getDataClient`.
- Node selection (найти подключённые часы).

### Сценарии
- Покупка на телефоне → отправить команду на часы (обновить питомца).
- Запись удара на часах → синхронизировать инвентарь.
- Egg hatching уведомление — оба экрана.

---

## 6. CORE BRIDGE (`dart:ffi`)

```dart
// core_bindings.dart
import 'dart:ffi';
import 'dart:io';
import 'package:ffi/ffi.dart';

typedef _QualityScoreNative = Float Function(Pointer<PunchMetricsC>, Pointer<HeartRateEvidenceC>);
typedef _QualityScore = double Function(Pointer<PunchMetricsC>, Pointer<HeartRateEvidenceC>);

class Core {
  late final DynamicLibrary _lib;
  late final _QualityScore _qualityScore;

  Core() {
    _lib = DynamicLibrary.open(
      Platform.isIOS ? 'GOCHYACore.framework/GOCHYACore' : 'libgochya_core.so',
    );
    _qualityScore = _lib.lookupFunction<_QualityScoreNative, _QualityScore>('gochya_quality_score');
  }

  double qualityScore(PunchMetrics m, HeartRateEvidence h) {
    return using((arena) {
      final mp = arena<PunchMetricsC>()..ref = m.toC(arena);
      final hp = arena<HeartRateEvidenceC>()..ref = h.toC(arena);
      return _qualityScore(mp, hp);
    });
  }
}
```

- Marshalling Dart ↔ C — в `types.dart`.
- Все формулы — вызовы в ядро, не пересчитываются локально.

---

## 7. IAP

- `in_app_purchase` plugin для единого API.
- iOS: StoreKit 2.
- Android: Google Play Billing + (опц.) Galaxy Store IAP SDK.
- Серверная валидация чеков (см. `BACKEND.md`).
- Подписка «GOCHYA+» через тот же плагин.

---

## 8. HEALTH-DOSTUP

- `health` package (pub.dev).
- Запрос: `HealthDataType.STEPS`, `SLEEP_ASLEEP`, `ACTIVE_ENERGY_BURNED`, `HEART_RATE`, `WORKOUTS`.
- Тот же consent UX, что и на часах.
- Чтение — раз в час + при открытии.

---

## 9. PERFORMANCE BUDGET (Companion)

| Метрика | Цель |
|---|---|
| Cold start | ≤ 2 сек |
| FPS | 60 |
| Память | ≤ 400 МБ |
| Размер приложения | ≤ 80 МБ |
| Battery (foreground active) | норм для соцсети/игры |

---

## 10. ТЕСТЫ

- Unit-тесты для `Core` (детерминизм FFI-вызовов).
- Widget-тесты для главных экранов.
- Integration-тесты с мок-сервером.

---

## 11. СВЯЗАННЫЕ ДОКУМЕНТЫ

- `ARCHITECTURE.md` — общая картина.
- `docs/04-core/CORE_SPEC.md` — контракт ядра.
- `docs/06-art/UX_UI.md` — UI-гайдлайны.
- `docs/01-design/GDD.md` — магазин, экономика.
- `docs/02-mechanics/MECHANIC_BREEDING.md` — экран бридинга/родословной.
