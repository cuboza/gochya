import 'dart:convert';

import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import '../models/auth_models.dart';
import '../models/profile_models.dart';

class SessionTokens {
  const SessionTokens({
    required this.accessToken,
    required this.refreshToken,
    this.accessTokenExpiresAt,
    this.refreshTokenExpiresAt,
  });

  factory SessionTokens.fromAuthTokenPair(AuthTokenPair pair) {
    return SessionTokens(
      accessToken: pair.accessToken,
      refreshToken: pair.refreshToken,
      accessTokenExpiresAt: pair.accessTokenExpiresAt,
      refreshTokenExpiresAt: pair.refreshTokenExpiresAt,
    );
  }

  factory SessionTokens.fromJson(JsonMap json) {
    if (rangedInt(json, 'storageVersion', min: 2, max: 2) != 2) {
      throw const FormatException('unsupported session storage version');
    }
    final accessExpiresAt = optionalDateTime(json, 'accessTokenExpiresAt');
    final refreshExpiresAt = optionalDateTime(json, 'refreshTokenExpiresAt');
    if ((accessExpiresAt == null) != (refreshExpiresAt == null) ||
        (accessExpiresAt != null &&
            !refreshExpiresAt!.isAfter(accessExpiresAt))) {
      throw const FormatException('session expiry metadata is inconsistent');
    }
    return SessionTokens(
      accessToken: requiredString(json, 'accessToken'),
      refreshToken: requiredString(json, 'refreshToken'),
      accessTokenExpiresAt: accessExpiresAt,
      refreshTokenExpiresAt: refreshExpiresAt,
    );
  }

  final String accessToken;
  final String refreshToken;
  final DateTime? accessTokenExpiresAt;
  final DateTime? refreshTokenExpiresAt;

  JsonMap toJson() {
    _validate();
    return {
      'storageVersion': 2,
      'accessToken': accessToken,
      'refreshToken': refreshToken,
      if (accessTokenExpiresAt != null)
        'accessTokenExpiresAt': accessTokenExpiresAt!.toUtc().toIso8601String(),
      if (refreshTokenExpiresAt != null)
        'refreshTokenExpiresAt': refreshTokenExpiresAt!
            .toUtc()
            .toIso8601String(),
    };
  }

  void _validate() {
    if (accessToken.trim().isEmpty || refreshToken.trim().isEmpty) {
      throw ArgumentError('session tokens must not be empty');
    }
    if ((accessTokenExpiresAt == null) != (refreshTokenExpiresAt == null) ||
        (accessTokenExpiresAt != null &&
            !refreshTokenExpiresAt!.isAfter(accessTokenExpiresAt!))) {
      throw ArgumentError('session expiry metadata is inconsistent');
    }
  }
}

abstract interface class SessionStore {
  Future<SessionTokens?> read();

  Future<void> write(SessionTokens tokens);

  Future<void> clear();
}

class SecureSessionStore implements SessionStore {
  SecureSessionStore({FlutterSecureStorage? storage})
    : _storage = storage ?? const FlutterSecureStorage();

  static const _sessionKey = 'gochya.session.v2';
  static const _legacyAccessTokenKey = 'gochya.access_token';
  static const _legacyRefreshTokenKey = 'gochya.refresh_token';

  final FlutterSecureStorage _storage;

  @override
  Future<SessionTokens?> read() async {
    final encoded = await _storage.read(key: _sessionKey);
    if (encoded != null) {
      try {
        final decoded = jsonDecode(encoded);
        return SessionTokens.fromJson(asMap(decoded, 'session'));
      } on FormatException {
        await clear();
        return null;
      } on ArgumentError {
        await clear();
        return null;
      }
    }
    return _migrateLegacySession();
  }

  @override
  Future<void> write(SessionTokens tokens) {
    final encoded = jsonEncode(tokens.toJson());
    return _storage.write(key: _sessionKey, value: encoded);
  }

  @override
  Future<void> clear() async {
    await Future.wait([
      _storage.delete(key: _sessionKey),
      _storage.delete(key: _legacyAccessTokenKey),
      _storage.delete(key: _legacyRefreshTokenKey),
    ]);
  }

  Future<SessionTokens?> _migrateLegacySession() async {
    final values = await Future.wait([
      _storage.read(key: _legacyAccessTokenKey),
      _storage.read(key: _legacyRefreshTokenKey),
    ]);
    final accessToken = values[0];
    final refreshToken = values[1];
    if (accessToken == null || refreshToken == null) {
      if (accessToken != null || refreshToken != null) {
        await clear();
      }
      return null;
    }
    final tokens = SessionTokens(
      accessToken: accessToken,
      refreshToken: refreshToken,
    );
    try {
      await write(tokens);
      await Future.wait([
        _storage.delete(key: _legacyAccessTokenKey),
        _storage.delete(key: _legacyRefreshTokenKey),
      ]);
      return tokens;
    } on Object {
      await clear();
      rethrow;
    }
  }
}
