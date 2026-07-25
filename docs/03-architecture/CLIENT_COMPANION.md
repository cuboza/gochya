# CLIENT: Телефон (Flutter) — полноценный клиент

> **Проектное решение: телефон — самодостаточная игра, а не companion.** На Android
> и iPhone доступен **весь цикл без часов**: питомец, уход, тренировки-миниигры,
> симбиоз с активностью, PvP, бридинг, экономика. Часы — **опциональный премиум-опыт**
> поверх (главное, что они добавляют, — запись удара в Dojo, §12).
>
> Раньше этот документ описывал телефон как «companion для тяжёлых экранов». Это
> изменено: телефонный игрок — **основная аудитория** (рынок телефонов несопоставимо
> больше рынка часов), и он проходит игру целиком. Тяжёлые экраны (магазин, гача,
> родословная, турниры) по-прежнему живут здесь, но теперь это **не всё** приложение,
> а его часть.

## 0. ЧТО РАБОТАЕТ БЕЗ ЧАСОВ, А ЧТО ТРЕБУЕТ ИХ

| Механика | Телефон без часов | Часы добавляют |
|---|---|---|
| Питомец, уход (корм/чистка/сон/игра) | ✅ полностью | быстрый доступ с запястья |
| Тренировки-миниигры (STR/AGI/END/FOC) | ✅ полностью | — |
| Симбиоз (Vitality, статы от активности) | ✅ шаги/сон/тренировки с телефона | точнее: пульс-зоны, сон с запястья (`MECHANIC_SYNERGY.md` §10) |
| **Dojo (запись удара)** | ❌ **только на часах** | ✅ **эксклюзив** — нужен акселерометр на запястье + пульс |
| Technique Cards | ✅ игровая добыча (Common…Epic) | ✅ + запись → Legendary/Mythic + signature |
| PvP (casual/ranked/сезоны) | ✅ полностью, единый пул с часами | — |
| Бридинг, инкубатор, родословная | ✅ полностью | — |
| Магазин, гача, Battle Pass | ✅ полностью | покупки на часах идут через телефон |

Ключевой разрыв — **Dojo**. Как телефонный игрок получает боевые карты без записи
и почему это не pay-to-win — `MECHANIC_COMBAT_RECORDING.md` §0 и `BALANCE.md` §5.3.

---

## 1. ТЕХНОЛОГИЧЕСКИЙ СТЕК

| Компонент | Технология |
|---|---|
| Фреймворк | Flutter 3.44.8 stable |
| Язык | Dart 3.12+ |
| State management | Riverpod |
| Routing | `Navigator` в первом срезе; `go_router` при появлении deep links |
| Сеть | типизированный boundary на `package:http`; без формул на клиенте |
| Realtime | web_socket_channel |
| Локальная БД | Drift (SQLite) или Isar |
| Health (iOS) | healthKit через `health` package |
| Health (Android) | Health Connect (основной агрегатор); Google Fit API не использовать для новых пользователей |
| IAP (iOS) | StoreKit 2 via `in_app_purchase` |
| IAP (Android) | Google Play Billing + Galaxy Store IAP |
| Core-интеграция | FFI (`dart:ffi`) к Shared Core |

Текущий исполняемый срез в `clients/companion` уже содержит Android/iOS runners,
Material app shell, Keychain/Keystore session store и read-only
`profile → pets → lineage` flow. JSON декодируется в строгие доменные модели:
невозможные needs, mutation mask за пределами 14 бит и повреждённые пары родителей
не маскируются в UI, а переходят в безопасное состояние ошибки. Production API
допускается только по HTTPS; HTTP оставлен только для loopback-разработки.
Телефонный onboarding подключён к server-authoritative age gate и starter egg:
клиент генерирует UUID idempotency keys, не сохраняет дату рождения, возобновляет
инкубацию через `GET /v1/me/eggs` и вызывает серверное вылупление. Категория
`under13` останавливается fail-closed до реализации проверяемого parental consent.
Первый care-срез отправляет Feed с `apple`, Clean, Play и Sleep в
`POST /v1/sync/commands`: `If-Match` привязан к `careRevision`, случайный
installation `deviceId` хранится в Keychain/Keystore, а неопределённый сетевой
результат повторяется с полностью тем же intent. Care intent сначала сохраняется
в account-bound encrypted journal в Keychain/Keystore и только затем уходит в
сеть. Журнал хранит до 100 команд, глобальный sequence и ожидаемые
revision; reconcile сохраняет порядок, разбивает поток на серверные batch одного
питомца и удаляет только терминально подтверждённые операции. `RETRYABLE`
остаётся в очереди, logout/смена `player.id` очищает старый журнал, а повреждённый
payload не затирается автоматически. После определённого ответа клиент
перечитывает canonical profile/pet snapshot; формул потребностей в Dart нет.
Вкладка Shop читает авторитетные `GET /v1/shop` и `GET /v1/me/items`, показывает
серверные Koins/цены и отправляет в `POST /v1/shop/buy` только `itemId`,
`quantity` и UUID `Idempotency-Key`. Результат применяется исключительно из
`PurchaseResponse`. Неопределённый сетевой исход блокирует следующие покупки до
canonical refresh каталога и инвентаря; клиент не ставит денежные операции в
offline-очередь и не повторяет их с новым ключом.
Authenticated profile, onboarding и care boundaries используют общий
single-flight refresh runner: первый `401` вызывает одну rotation, параллельные
запросы ждут её, а затем каждый intent повторяется ровно один раз. Новые access
и refresh tokens вместе со сроками сохраняются одним versioned документом
Keychain/Keystore; прежние два ключа мигрируют при чтении. `refresh_token_invalid`,
повторный `401`, ошибка локальной записи и неопределённый исход refresh очищают
сессию и account-bound очередь fail-closed. Старый refresh после потери ответа
не повторяется: backend уже мог его потребить, и retry вызвал бы reuse detection
для всей family. Android использует нативный Google Sign-In: Web OAuth client ID
передаётся через compile-time define, SDK выдаёт ID token, а клиент обменивает его
на GOCHYA access/refresh pair через `POST /v1/auth/google`. Email и provider user
ID не используются как доказательство личности, токены Google не записываются в
session store, а без client ID кнопка fail-closed скрыта. iOS Google OAuth пока
не включён без обязательного статического URL scheme. Вместо него iOS использует
нативный Sign in with Apple с app capability: клиент получает одноразовый nonce
из `POST /v1/auth/apple/preflight`, без преобразования устанавливает его в
AuthenticationServices request и отправляет тот же nonce с Apple identity token
в `POST /v1/auth/apple`. Email/full name scopes не запрашиваются, Apple credential
не хранится, GOCHYA-сессия появляется только после backend verification.
Недоступный native provider и истёкший nonce обрабатываются fail-closed.
Явный logout best-effort вызывает `POST /v1/auth/logout` до локальной очистки,
чтобы отозвать server-side token family, но не удерживает refresh token на
устройстве при офлайне: ожидание ограничено тремя секундами, после чего session
store, provider state и account-bound очередь стираются. Монотонное поколение
сессии инвалидирует уже начатые refresh-операции; поздний ответ не может
восстановить вышедшую сессию или выполнить старый intent под новым аккаунтом.
Shared Core FFI остаётся следующим срезом.

---

## 2. ПОЧЕМУ FLUTTER

- Один кодбейз под iOS и Android.
- Shared Core через `dart:ffi` — те же формулы, что на часах. **Критично: раз телефон
  проходит игру целиком, ядро на нём исполняет ту же логику ухода/боя/симбиоза, что
  и часы, — не дублируя формул** (`00-MASTER-PROMPT.md` §2.1).
- Богатая UI-библиотека для питомца, магазина, родословной, графиков.
- Быстрый iteration cycle.
- Хорошая производительность (60 FPS на современных телефонах не проблема) — важно
  для HD-рендера питомца, который здесь основной экран, а не превью.

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
│   │   ├── home/                      ← дашборд, питомец HD (главный экран)
│   │   ├── pet/                       ← детальный профиль питомца
│   │   ├── care/                      ← кормить/чистить/спать/играть (полноценно, не превью)
│   │   ├── training/                  ← мини-игры тренировок STR/AGI/END/FOC
│   │   ├── shop/                      ← магазин, разделы
│   │   ├── gacha/                     ← гача-баннеры (источник карт на телефоне)
│   │   ├── inventory/                 ← инвентарь, экипировка, Technique Cards
│   │   ├── breeding/                  ← бридинг, инкубатор
│   │   ├── lineage/                   ← родословная (дерево)
│   │   ├── pvp/                       ← дуэли, лоадаут, единый пул с часами
│   │   ├── tournaments/               ← чемпионаты, бракеты
│   │   ├── leaderboard/               ← рейтинги, лиги
│   │   ├── market/                    ← P2P рынок (фаза 2)
│   │   ├── battle_pass/               ← сезонный пропуск
│   │   ├── friends/                   ← друзья, спарринг
│   │   ├── activity/                  ← кольца, журналы, графики
│   │   ├── dojo_upsell/               ← экран «подключи часы для записи ударов» (§12)
│   │   └── settings/
│   ├── core/                          ← FFI bridge к Shared Core
│   │   ├── core_bindings.dart         ← dart:ffi
│   │   ├── types.dart                 ← Dart ↔ C-structs
│   │   └── core_initializer.dart
│   ├── services/
│   │   ├── api_client.dart            ← dio + retrofit
│   │   ├── auth_service.dart
│   │   ├── health_service.dart        ← HealthKit / Health Connect
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
        └── libgochya_core.so           ← Shared Core, ТОЛЬКО Android
                                        (iOS линкуется статически из .a — см. §6,
                                         .dylib нельзя: App Store запрещает dlopen)
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
- В первом исполняемом срезе доступны серверные care/breeding SKU за Koins и
  приватный инвентарь; расширенные валюты и категории остаются следующими
  контрактами.

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
- Клиент строит дерево из нормализованного ответа
  `GET /v1/me/pets/:id/lineage`: `nodes` индексируются по `id`, связи берутся
  только из `parentAId`/`parentBId`. `maxDepth=3` — серверная граница, а
  `truncated=true` показывает UI-маркер продолжения без дополнительного
  неограниченного обхода.

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

### Режим без часов (основной для большинства игроков)
- Часы **не** требуются для запуска и прохождения игры. `watch_connection.dart`
  при отсутствии спаренных часов работает в no-op режиме, UI не показывает
  «ошибку подключения» — отсутствие часов это норма, а не сбой.
- Состояние — авторитетно на сервере (`ARCHITECTURE.md` §8), поэтому телефон и часы
  видят одного питомца; какой клиент активен, роли не играет.
- Единственное, чего нет без часов, — экран Dojo. Вместо него телефон показывает
  `dojo_upsell` (§12).

## 5а. DOJO-АПСЕЛЛ (экран «зачем нужны часы»)

Раз запись удара — эксклюзив часов, телефон должен **объяснить ценность**, не создавая
ощущения ущербности:
- Показать, что даёт запись: карты Legendary/Mythic «твоего стиля», spirit-бонус,
  персональный signature-приём (`MECHANIC_COMBAT_RECORDING.md` §0).
- Подчеркнуть, что телефонный игрок **уже конкурентоспособен**: карты Epic из игровой
  добычи + бридинг + снаряжение достаточно для любой лиги (`BALANCE.md` §5.3).
- CTA: «Есть Galaxy Watch или Apple Watch? Подключи и записывай приёмы» — без блокировок
  и таймеров. Никаких «купи часы, иначе проиграешь».

---

## 6. CORE BRIDGE (`dart:ffi`)

> ⚠️ **Аудит T3 (критично):** прежний код использовал `DynamicLibrary.open('GOCHYACore.framework/...')` на iOS — это **запрещено** App Store (запрет `dlopen` для сторонних фреймворков; пройдёт debug, упадёт в release/TestFlight/App Review). На iOS нужно **статически линковать** Rust staticlib через Flutter plugin и использовать `DynamicLibrary.process()` (поиск символов в главном бинарнике).

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
    // audit T3: iOS — статическая линковка, символы в главном бинарнике
    // Android — динамическая загрузка .so
    _lib = Platform.isIOS
        ? DynamicLibrary.process()                                  // ← НЕ .open()!
        : DynamicLibrary.open('libgochya_core.so');
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

### Сборка iOS (audit T3)
1. Rust: `cargo build --release --target aarch64-apple-ios` → `libgochya_core.a` (staticlib).
2. Создать Flutter **plugin** (CocoaPods), который линкует `libgochya_core.a` статически в нативную часть плагина.
3. В `ios/gochya_core.podspec` указать `s.vendored_libraries = 'Libs/libgochya_core.a'` и `s.libraries = 'c++'`.
4. В итоге символы ядра попадают в главный бинарник приложения → `DynamicLibrary.process()` их находит.
5. **Не** использовать embedded dynamic framework — это нарушит App Review.

### Сборка Android
1. Rust: `cargo build --release --target aarch64-linux-android` → `libgochya_core.so`.
2. Поместить в `android/app/src/main/jniLibs/arm64-v8a/libgochya_core.so`.
3. `DynamicLibrary.open('libgochya_core.so')` загружает в рантайме (разрешено на Android).

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
