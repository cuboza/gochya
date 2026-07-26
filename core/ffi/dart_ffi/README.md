# Dart FFI bridge

Bindings живут в клиенте: `clients/companion/lib/core/ffi/`.

- `core_types.dart` — versioned C-структуры, статусы, строгие value-типы.
- `core_bindings.dart` — разрешение библиотеки по платформам и lookup символов.
- `gochya_core.dart` — типизированный API.

Контракт и сборка — `docs/03-architecture/CLIENT_COMPANION.md` §6.
