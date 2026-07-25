import 'dart:convert';

import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gochya_companion/core/session/session_store.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  test('stores a rotated token pair as one protected document', () async {
    FlutterSecureStorage.setMockInitialValues({});
    const storage = FlutterSecureStorage();
    final store = SecureSessionStore(storage: storage);
    final tokens = SessionTokens(
      accessToken: 'access',
      refreshToken: 'refresh',
      accessTokenExpiresAt: DateTime.utc(2026, 7, 25, 12, 15),
      refreshTokenExpiresAt: DateTime.utc(2026, 8, 24, 12),
    );

    await store.write(tokens);

    final values = await storage.readAll();
    expect(values.keys, ['gochya.session.v2']);
    final encoded = values['gochya.session.v2'];
    expect(encoded, isNotNull);
    final json = jsonDecode(encoded!) as Map<String, dynamic>;
    expect(json['storageVersion'], 2);
    expect(json['accessToken'], 'access');
    expect(json['refreshToken'], 'refresh');

    final restored = await store.read();
    expect(restored?.accessToken, tokens.accessToken);
    expect(restored?.refreshToken, tokens.refreshToken);
    expect(restored?.accessTokenExpiresAt, tokens.accessTokenExpiresAt);
    expect(restored?.refreshTokenExpiresAt, tokens.refreshTokenExpiresAt);
  });

  test('migrates the two-key legacy session into one document', () async {
    FlutterSecureStorage.setMockInitialValues({
      'gochya.access_token': 'legacy-access',
      'gochya.refresh_token': 'legacy-refresh',
    });
    const storage = FlutterSecureStorage();
    final store = SecureSessionStore(storage: storage);

    final restored = await store.read();

    expect(restored?.accessToken, 'legacy-access');
    expect(restored?.refreshToken, 'legacy-refresh');
    final values = await storage.readAll();
    expect(values.keys, ['gochya.session.v2']);
  });
}
