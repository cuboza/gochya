# GOCHYA для телефона

Flutter-клиент для Android и iOS. Первый исполняемый срез содержит:

- Material 3 app shell с пятью основными разделами;
- безопасное хранение access/refresh tokens в Keychain/Keystore;
- типизированные клиенты `GET /v1/me`, `GET /v1/me/pets` и
  `GET /v1/me/pets/:id/lineage`;
- главную активного питомца с четырьмя потребностями;
- bounded lineage до трёх поколений;
- fail-closed состояния повреждённого ответа, недоступной сессии и HTTP 401.

OAuth UI намеренно не имитируется: до подключения provider credentials приложение
показывает честное состояние без сессии. Ручного ввода bearer token в production UI
нет.

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
flutter analyze
flutter test
```

CI дополнительно собирает debug APK и iOS Simulator application. Production build
обязан передавать HTTPS `GOCHYA_API_BASE_URL`.
