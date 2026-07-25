import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:gochya_companion/core/device/installation_id_store.dart';
import 'package:gochya_companion/core/network/gochya_api_client.dart';
import 'package:gochya_companion/features/auth/apple_identity_client.dart';
import 'package:gochya_companion/features/auth/auth_repository.dart';
import 'package:gochya_companion/features/auth/google_identity_client.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

void main() {
  group('ApiAuthRepository', () {
    test(
      'exchanges only provider proof and returns complete session',
      () async {
        late http.Request request;
        final identity = _IdentityClient();
        final repository = ApiAuthRepository(
          apiClient: GochyaApiClient(
            baseUri: Uri.parse('https://api.example.test'),
            httpClient: MockClient((value) async {
              request = value;
              return _jsonResponse({
                'jwt': 'issued-access',
                'refreshToken': 'issued-refresh',
                'accessTokenExpiresAt': '2026-07-25T10:15:00Z',
                'refreshTokenExpiresAt': '2026-08-24T10:00:00Z',
                'player': {
                  'id': 'player-1',
                  'username': 'nika',
                  'displayName': 'Ника',
                },
              }, 200);
            }),
          ),
          appleIdentityClient: _AppleIdentityClient(),
          identityClient: identity,
          installationIdStore: const _InstallationIdStore(),
        );

        final tokens = await repository.signInWithGoogle();

        expect(jsonDecode(request.body), {
          'idToken': 'google-id-token',
          'deviceId': '11111111-1111-4111-8111-111111111111',
        });
        expect(tokens.accessToken, 'issued-access');
        expect(tokens.refreshToken, 'issued-refresh');
        expect(
          tokens.accessTokenExpiresAt,
          DateTime.parse('2026-07-25T10:15:00Z'),
        );
        expect(identity.signOutCalls, 0);
      },
    );

    test('cleans provider state when backend exchange fails', () async {
      final identity = _IdentityClient();
      final repository = ApiAuthRepository(
        apiClient: GochyaApiClient(
          baseUri: Uri.parse('https://api.example.test'),
          httpClient: MockClient((request) async {
            return _jsonResponse({
              'error': {
                'code': 'identity_token_invalid',
                'message': 'invalid identity token',
                'request_id': 'request-1',
              },
            }, 401);
          }),
        ),
        appleIdentityClient: _AppleIdentityClient(),
        identityClient: identity,
        installationIdStore: const _InstallationIdStore(),
      );

      await expectLater(
        repository.signInWithGoogle(),
        throwsA(
          isA<ApiException>().having(
            (error) => error.code,
            'code',
            'identity_token_invalid',
          ),
        ),
      );
      expect(identity.signOutCalls, 1);
    });

    test('does not touch backend when provider login is cancelled', () async {
      var requestCount = 0;
      final repository = ApiAuthRepository(
        apiClient: GochyaApiClient(
          baseUri: Uri.parse('https://api.example.test'),
          httpClient: MockClient((request) async {
            requestCount++;
            return _jsonResponse(<String, dynamic>{}, 500);
          }),
        ),
        appleIdentityClient: _AppleIdentityClient(),
        identityClient: _IdentityClient(
          error: const GoogleIdentityException(GoogleIdentityFailure.cancelled),
        ),
        installationIdStore: const _InstallationIdStore(),
      );

      await expectLater(
        repository.signInWithGoogle(),
        throwsA(
          isA<GoogleIdentityException>().having(
            (error) => error.failure,
            'failure',
            GoogleIdentityFailure.cancelled,
          ),
        ),
      );
      expect(requestCount, 0);
    });

    test('binds one server nonce to native Apple and backend calls', () async {
      final requests = <http.Request>[];
      final appleIdentity = _AppleIdentityClient();
      final repository = ApiAuthRepository(
        apiClient: GochyaApiClient(
          baseUri: Uri.parse('https://api.example.test'),
          httpClient: MockClient((request) async {
            requests.add(request);
            if (request.url.path.endsWith('/preflight')) {
              return _jsonResponse({
                'nonce': 'server-apple-nonce',
                'expiresAt': '2026-07-25T10:05:00Z',
              }, 200);
            }
            return _jsonResponse({
              'jwt': 'apple-access',
              'refreshToken': 'apple-refresh',
              'accessTokenExpiresAt': '2026-07-25T10:15:00Z',
              'refreshTokenExpiresAt': '2026-08-24T10:00:00Z',
              'player': {'id': 'player-1', 'username': 'apple-player'},
            }, 200);
          }),
        ),
        appleIdentityClient: appleIdentity,
        identityClient: _IdentityClient(),
        installationIdStore: const _InstallationIdStore(),
      );

      final tokens = await repository.signInWithApple();

      expect(appleIdentity.seenNonce, 'server-apple-nonce');
      expect(requests.first.body, isEmpty);
      expect(jsonDecode(requests.last.body), {
        'identityToken': 'apple-identity-token',
        'nonce': 'server-apple-nonce',
        'deviceId': '11111111-1111-4111-8111-111111111111',
      });
      expect(tokens.accessToken, 'apple-access');
      expect(tokens.refreshToken, 'apple-refresh');
    });

    test(
      'does not allocate an Apple nonce when provider is unavailable',
      () async {
        var requestCount = 0;
        final repository = ApiAuthRepository(
          apiClient: GochyaApiClient(
            baseUri: Uri.parse('https://api.example.test'),
            httpClient: MockClient((request) async {
              requestCount++;
              return _jsonResponse(<String, dynamic>{}, 500);
            }),
          ),
          appleIdentityClient: _AppleIdentityClient(available: false),
          identityClient: _IdentityClient(),
          installationIdStore: const _InstallationIdStore(),
        );

        await expectLater(
          repository.signInWithApple(),
          throwsA(
            isA<AppleIdentityException>().having(
              (error) => error.failure,
              'failure',
              AppleIdentityFailure.unavailable,
            ),
          ),
        );
        expect(requestCount, 0);
      },
    );
  });
}

class _AppleIdentityClient implements AppleIdentityClient {
  _AppleIdentityClient({this.available = true});

  final bool available;
  String? seenNonce;

  @override
  Future<String> authenticateIdentityToken(String nonce) async {
    seenNonce = nonce;
    return 'apple-identity-token';
  }

  @override
  Future<bool> isAvailable() async => available;
}

class _IdentityClient implements GoogleIdentityClient {
  _IdentityClient({this.error});

  final Object? error;
  int signOutCalls = 0;

  @override
  bool get isAvailable => true;

  @override
  Future<String> authenticateIdToken() async {
    if (error case final error?) {
      throw error;
    }
    return 'google-id-token';
  }

  @override
  Future<void> signOut() async {
    signOutCalls++;
  }
}

class _InstallationIdStore implements InstallationIdStore {
  const _InstallationIdStore();

  @override
  Future<String> getOrCreate() async {
    return '11111111-1111-4111-8111-111111111111';
  }
}

http.Response _jsonResponse(Object body, int statusCode) {
  return http.Response(
    jsonEncode(body),
    statusCode,
    headers: {'content-type': 'application/json; charset=utf-8'},
  );
}
