import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/network/api_providers.dart';
import '../../core/network/gochya_api_client.dart';
import '../../core/session/session_store.dart';
import 'session_controller.dart';

final sessionRequestRunnerProvider = Provider<SessionRequestRunner>(
  (ref) => SessionRequestRunner(
    api: ref.watch(apiClientProvider),
    sessionStore: ref.watch(sessionStoreProvider),
    sessionLifecycle: ref.read(sessionControllerProvider.notifier),
  ),
);

abstract interface class AuthenticatedRequestRunner {
  Future<T> run<T>({
    required String accessToken,
    required Future<T> Function(String accessToken) request,
  });
}

class SessionRequestRunner implements AuthenticatedRequestRunner {
  SessionRequestRunner({
    required this.api,
    required this.sessionStore,
    required this.sessionLifecycle,
  });

  final GochyaApiClient api;
  final SessionStore sessionStore;
  final SessionLifecycle sessionLifecycle;

  Future<SessionTokens>? _refreshInFlight;

  @override
  Future<T> run<T>({
    required String accessToken,
    required Future<T> Function(String accessToken) request,
  }) async {
    final expectedGeneration = sessionLifecycle.sessionGeneration;
    try {
      return await request(accessToken);
    } on ApiException catch (error) {
      if (!error.isUnauthorized) {
        rethrow;
      }
    }

    final rotated = await _refreshAfterUnauthorized(
      accessToken,
      expectedGeneration,
    );
    if (expectedGeneration != sessionLifecycle.sessionGeneration) {
      throw _sessionSuperseded();
    }
    try {
      return await request(rotated.accessToken);
    } on ApiException catch (error) {
      if (error.isUnauthorized) {
        await sessionLifecycle.expireSession();
      }
      rethrow;
    }
  }

  Future<SessionTokens> _refreshAfterUnauthorized(
    String rejectedAccessToken,
    int expectedGeneration,
  ) async {
    if (expectedGeneration != sessionLifecycle.sessionGeneration) {
      throw _sessionSuperseded();
    }
    final current = await sessionStore.read();
    if (expectedGeneration != sessionLifecycle.sessionGeneration) {
      throw _sessionSuperseded();
    }
    if (current == null) {
      throw const ApiException(
        statusCode: 401,
        code: 'session_unavailable',
        message: 'No refreshable session is available.',
      );
    }
    if (current.accessToken != rejectedAccessToken) {
      return current;
    }

    final activeRefresh = _refreshInFlight;
    if (activeRefresh != null) {
      return activeRefresh;
    }
    final refresh = _rotate(current, expectedGeneration);
    _refreshInFlight = refresh;
    try {
      return await refresh;
    } finally {
      if (identical(_refreshInFlight, refresh)) {
        _refreshInFlight = null;
      }
    }
  }

  Future<SessionTokens> _rotate(
    SessionTokens current,
    int expectedGeneration,
  ) async {
    late final SessionTokens rotated;
    late final bool accepted;
    try {
      final pair = await api.refreshSession(current.refreshToken);
      rotated = SessionTokens.fromAuthTokenPair(pair);
      accepted = await sessionLifecycle.replaceAfterRefresh(
        rotated,
        expectedGeneration: expectedGeneration,
      );
    } on Object {
      // A refresh response can be lost after the server has consumed the
      // one-time token. Retrying that token risks reuse detection and family
      // revocation, so every uncertain refresh outcome fails closed.
      await sessionLifecycle.expireSession();
      rethrow;
    }
    if (!accepted) {
      throw _sessionSuperseded();
    }
    return rotated;
  }

  ApiException _sessionSuperseded() {
    return const ApiException(
      statusCode: 401,
      code: 'session_superseded',
      message: 'The session changed while the request was in progress.',
    );
  }
}
