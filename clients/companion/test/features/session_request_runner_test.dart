import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:gochya_companion/core/models/auth_models.dart';
import 'package:gochya_companion/core/network/gochya_api_client.dart';
import 'package:gochya_companion/core/session/session_store.dart';
import 'package:gochya_companion/features/session/session_controller.dart';
import 'package:gochya_companion/features/session/session_request_runner.dart';

void main() {
  test('parallel unauthorized requests share one refresh rotation', () async {
    final store = _MemorySessionStore(_oldTokens);
    final lifecycle = _MemorySessionLifecycle(store);
    final api = _RefreshApi();
    final runner = SessionRequestRunner(
      api: api,
      sessionStore: store,
      sessionLifecycle: lifecycle,
    );
    var oldRequests = 0;
    var rotatedRequests = 0;

    Future<String> request(String accessToken) async {
      if (accessToken == _oldTokens.accessToken) {
        oldRequests++;
        throw const ApiException(
          statusCode: 401,
          code: 'token_expired',
          message: 'expired',
        );
      }
      rotatedRequests++;
      return accessToken;
    }

    final first = runner.run(
      accessToken: _oldTokens.accessToken,
      request: request,
    );
    final second = runner.run(
      accessToken: _oldTokens.accessToken,
      request: request,
    );
    await api.refreshStarted.future;
    expect(api.refreshCalls, 1);
    api.completeRefresh(_rotatedPair);

    expect(await Future.wait([first, second]), [
      _rotatedPair.accessToken,
      _rotatedPair.accessToken,
    ]);
    expect(oldRequests, 2);
    expect(rotatedRequests, 2);
    expect(lifecycle.replacements, 1);
    expect(store.tokens?.refreshToken, _rotatedPair.refreshToken);

    final late = await runner.run(
      accessToken: _oldTokens.accessToken,
      request: request,
    );
    expect(late, _rotatedPair.accessToken);
    expect(api.refreshCalls, 1);
  });

  test('uncertain refresh failure clears the local token family', () async {
    final store = _MemorySessionStore(_oldTokens);
    final lifecycle = _MemorySessionLifecycle(store);
    final api = _RefreshApi(
      refreshError: const ApiException(
        code: 'network_error',
        message: 'response lost',
      ),
    );
    final runner = SessionRequestRunner(
      api: api,
      sessionStore: store,
      sessionLifecycle: lifecycle,
    );
    var requestCount = 0;

    await expectLater(
      runner.run<void>(
        accessToken: _oldTokens.accessToken,
        request: (accessToken) async {
          requestCount++;
          throw const ApiException(
            statusCode: 401,
            code: 'token_expired',
            message: 'expired',
          );
        },
      ),
      throwsA(
        isA<ApiException>().having(
          (error) => error.code,
          'code',
          'network_error',
        ),
      ),
    );

    expect(requestCount, 1);
    expect(lifecycle.expirations, 1);
    expect(store.tokens, isNull);
  });

  test('unauthorized retry with a rotated token expires the session', () async {
    final store = _MemorySessionStore(_oldTokens);
    final lifecycle = _MemorySessionLifecycle(store);
    final api = _RefreshApi();
    final runner = SessionRequestRunner(
      api: api,
      sessionStore: store,
      sessionLifecycle: lifecycle,
    );

    final result = runner.run<void>(
      accessToken: _oldTokens.accessToken,
      request: (accessToken) async {
        throw const ApiException(
          statusCode: 401,
          code: 'token_expired',
          message: 'expired',
        );
      },
    );
    await api.refreshStarted.future;
    api.completeRefresh(_rotatedPair);

    await expectLater(result, throwsA(isA<ApiException>()));
    expect(lifecycle.replacements, 1);
    expect(lifecycle.expirations, 1);
    expect(store.tokens, isNull);
  });
}

final _oldTokens = SessionTokens(
  accessToken: 'old-access',
  refreshToken: 'old-refresh',
  accessTokenExpiresAt: DateTime.utc(2026, 7, 25, 12, 15),
  refreshTokenExpiresAt: DateTime.utc(2026, 8, 24, 12),
);

final _rotatedPair = AuthTokenPair(
  accessToken: 'rotated-access',
  refreshToken: 'rotated-refresh',
  accessTokenExpiresAt: DateTime.utc(2026, 7, 25, 12, 30),
  refreshTokenExpiresAt: DateTime.utc(2026, 8, 24, 12, 15),
);

class _RefreshApi extends GochyaApiClient {
  _RefreshApi({this.refreshError})
    : super(baseUri: Uri.parse('https://api.example.com'));

  final Object? refreshError;
  final Completer<void> refreshStarted = Completer<void>();
  final Completer<AuthTokenPair> _refreshResult = Completer<AuthTokenPair>();
  int refreshCalls = 0;

  @override
  Future<AuthTokenPair> refreshSession(String refreshToken) {
    refreshCalls++;
    expect(refreshToken, _oldTokens.refreshToken);
    if (!refreshStarted.isCompleted) {
      refreshStarted.complete();
    }
    final error = refreshError;
    if (error != null) {
      return Future.error(error);
    }
    return _refreshResult.future;
  }

  void completeRefresh(AuthTokenPair pair) {
    _refreshResult.complete(pair);
  }
}

class _MemorySessionStore implements SessionStore {
  _MemorySessionStore(this.tokens);

  SessionTokens? tokens;

  @override
  Future<void> clear() async {
    tokens = null;
  }

  @override
  Future<SessionTokens?> read() async => tokens;

  @override
  Future<void> write(SessionTokens tokens) async {
    this.tokens = tokens;
  }
}

class _MemorySessionLifecycle implements SessionLifecycle {
  _MemorySessionLifecycle(this.store);

  final _MemorySessionStore store;
  int replacements = 0;
  int expirations = 0;

  @override
  Future<void> expireSession() async {
    expirations++;
    await store.clear();
  }

  @override
  Future<void> replaceAfterRefresh(SessionTokens tokens) async {
    replacements++;
    await store.write(tokens);
  }
}
