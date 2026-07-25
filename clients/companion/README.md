# GOCHYA для телефона

Flutter-клиент для Android и iOS. Первый исполняемый срез содержит:

- Material 3 app shell с пятью основными разделами;
- безопасное хранение access/refresh tokens в Keychain/Keystore;
- типизированные клиенты `GET /v1/me`, `GET /v1/me/pets` и
  `GET /v1/me/pets/:id/lineage`;
- главную активного питомца с четырьмя потребностями;
- bounded lineage до трёх поколений;
- обязательный age gate, выбор Fire/Water/Earth starter-яйца, возобновляемую
  инкубацию и вылупление первого питомца;
- server-authoritative кормление яблоком, чистку, игру и сон через
  `POST /v1/sync/commands`, revision precondition и canonical refresh;
- fail-closed состояния повреждённого ответа, недоступной сессии и HTTP 401.

OAuth UI намеренно не имитируется: до подключения provider credentials приложение
показывает честное состояние без сессии. Ручного ввода bearer token в production UI
нет. Дата рождения отправляется только в age-gate запрос и не сохраняется
клиентом; `under13` не может продолжить, пока серверный parental-consent flow
не реализован.
Случайный installation `deviceId` хранится в Keychain/Keystore. Если ответ care
теряется в сети, UI повторяет исходный `operationId`, wall time и monotonic
offset, поэтому серверная дедупликация не допускает двойного эффекта. Текущий
срез выполняет немедленный online reconcile; зашифрованная persistent-очередь
для полноценной офлайн-игры ещё не подключена.

## Запуск

Требуется Flutter 3.44.8 stable или совместимая более новая stable-версия.

```bash
cd clients/companion
flutter pub get
flutter run \
  --dart-define=GOCHYA_API_BASE_URL=https://api.example.com
```

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
обязан передавать HTTPS `GOCHYA_API_BASE_URL`.
