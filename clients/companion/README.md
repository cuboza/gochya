# GOCHYA для телефона

Flutter-клиент для Android и iOS. Первый исполняемый срез содержит:

- Material 3 app shell с пятью основными разделами;
- нативный Google Sign-In на Android с обменом ID token через
  `POST /v1/auth/google`;
- нативный Sign in with Apple на iOS с одноразовым server nonce через
  `POST /v1/auth/apple/preflight` и `POST /v1/auth/apple`;
- безопасное хранение access/refresh pair одним versioned документом в
  Keychain/Keystore и single-flight rotation после `401`;
- типизированные клиенты `GET /v1/me`, `GET /v1/me/pets` и
  `GET /v1/me/pets/:id/lineage`;
- главную активного питомца с четырьмя потребностями;
- bounded lineage до трёх поколений;
- обязательный age gate, выбор Fire/Water/Earth starter-яйца, возобновляемую
  инкубацию и вылупление первого питомца;
- server-authoritative кормление яблоком, чистку, игру и сон через
  `POST /v1/sync/commands`, revision precondition и canonical refresh;
- account-bound encrypted offline-очередь до 100 care-команд с автоматическим
  последовательным batch reconcile после запуска;
- fail-closed состояния повреждённого ответа, недоступной сессии и
  невосстановимого HTTP 401.

На Android кнопка Google появляется только при переданном Web OAuth client ID.
Клиент отправляет backend полученный от нативного SDK ID token, а не email или
Google user ID; GOCHYA-сессия сохраняется только после успешной серверной
проверки. На iOS доступность Sign in with Apple проверяется нативным SDK перед
показом кнопки. Каждая попытка сначала получает одноразовый nonce backend,
передаёт его Apple без преобразования и возвращает тот же nonce с identity
token; email и имя не запрашиваются. Без доступного провайдера приложение
показывает честное состояние без сессии. Ручного ввода bearer token в production
UI нет. Дата рождения отправляется только в age-gate запрос и не сохраняется
клиентом; `under13` не может продолжить, пока серверный parental-consent flow
не реализован.
Случайный installation `deviceId` и care-журнал хранятся в Keychain/Keystore.
Команда попадает в журнал до HTTP-вызова, поэтому потеря ответа или перезапуск
приложения повторяет исходные `operationId`, wall time и monotonic offset без
двойного эффекта. Очередь привязана к `player.id`, очищается при logout или смене
аккаунта, сохраняет глобальный sequence и отправляет последовательные batch по
одному питомцу. Удаляются только команды с подтверждённым терминальным статусом;
`RETRYABLE` остаётся в защищённом хранилище. Клиент не рассчитывает локальный
эффект care и после ответа перечитывает canonical состояние.

Authenticated profile, onboarding и care-запросы при первом `401` выполняют одну
общую `POST /v1/auth/refresh` rotation и повторяются ровно один раз с новой
access/refresh pair. Новая пара вместе со сроками заменяет прежнюю одним
защищённым документом; двухключевое хранилище V1 мигрирует при чтении. Если
refresh token отклонён, повторный запрос снова получает `401`, локальная запись
новой пары не удалась или исход refresh неопределён из-за потери ответа, сессия
и account-bound care-очередь очищаются fail-closed. Старый одноразовый refresh
не повторяется, чтобы не вызвать server-side reuse revocation.

## Запуск

Требуется Flutter 3.44.8 stable или совместимая более новая stable-версия.

```bash
cd clients/companion
flutter pub get
flutter run \
  --dart-define=GOCHYA_API_BASE_URL=https://api.example.com \
  --dart-define=GOCHYA_GOOGLE_SERVER_CLIENT_ID=000000000000-example.apps.googleusercontent.com
```

Для Google Sign-In нужно зарегистрировать Android OAuth application для package
`com.gochya.gochya_companion` и SHA-1 каждого используемого signing certificate,
а в `GOCHYA_GOOGLE_SERVER_CLIENT_ID` передать Web application client ID,
разрешённый backend. Если define отсутствует, кнопка входа fail-closed скрыта.

Для iOS в Xcode project уже добавлены Sign in with Apple capability и
entitlements. В Apple Developer Account нужно включить capability для App ID
`com.gochya.gochyaCompanion`, обновить provisioning profiles, а backend запустить
с этим bundle ID в `GOCHYA_APPLE_CLIENT_IDS`. Никаких Apple private key или
client secret в приложение добавлять нельзя.

Для локального API на Android Emulator используется адрес хоста `10.0.2.2`:

```bash
flutter run \
  --dart-define=GOCHYA_API_BASE_URL=http://10.0.2.2:8080
```

HTTP разрешён клиентом только для loopback и стандартных адресов host loopback
Android Emulator; release-сборка Android не включает cleartext override.

## Проверка

```bash
dart format --output=none --set-exit-if-changed lib test
dart analyze lib test
flutter test
flutter build apk --debug \
  --dart-define=GOCHYA_API_BASE_URL=https://api.example.com
```

CI дополнительно собирает debug APK и iOS Simulator application. Production build
обязан передавать HTTPS `GOCHYA_API_BASE_URL`; Android-сборка с Google-входом —
также `GOCHYA_GOOGLE_SERVER_CLIENT_ID`.
