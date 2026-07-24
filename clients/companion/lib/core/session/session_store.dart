import 'package:flutter_secure_storage/flutter_secure_storage.dart';

class SessionTokens {
  const SessionTokens({required this.accessToken, required this.refreshToken});

  final String accessToken;
  final String refreshToken;
}

abstract interface class SessionStore {
  Future<SessionTokens?> read();

  Future<void> write(SessionTokens tokens);

  Future<void> clear();
}

class SecureSessionStore implements SessionStore {
  SecureSessionStore({FlutterSecureStorage? storage})
    : _storage = storage ?? const FlutterSecureStorage();

  static const _accessTokenKey = 'gochya.access_token';
  static const _refreshTokenKey = 'gochya.refresh_token';

  final FlutterSecureStorage _storage;

  @override
  Future<SessionTokens?> read() async {
    final values = await Future.wait([
      _storage.read(key: _accessTokenKey),
      _storage.read(key: _refreshTokenKey),
    ]);
    final accessToken = values[0];
    final refreshToken = values[1];
    if (accessToken == null || refreshToken == null) {
      if (accessToken != null || refreshToken != null) {
        await clear();
      }
      return null;
    }
    if (accessToken.trim().isEmpty || refreshToken.trim().isEmpty) {
      await clear();
      return null;
    }
    return SessionTokens(accessToken: accessToken, refreshToken: refreshToken);
  }

  @override
  Future<void> write(SessionTokens tokens) async {
    if (tokens.accessToken.trim().isEmpty ||
        tokens.refreshToken.trim().isEmpty) {
      throw ArgumentError('session tokens must not be empty');
    }
    await _storage.write(key: _accessTokenKey, value: tokens.accessToken);
    try {
      await _storage.write(key: _refreshTokenKey, value: tokens.refreshToken);
    } on Object {
      await clear();
      rethrow;
    }
  }

  @override
  Future<void> clear() async {
    await Future.wait([
      _storage.delete(key: _accessTokenKey),
      _storage.delete(key: _refreshTokenKey),
    ]);
  }
}
