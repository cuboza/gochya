import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/session/session_store.dart';
import '../auth/auth_repository.dart';
import '../care/care_queue_store.dart';

final sessionStoreProvider = Provider<SessionStore>(
  (ref) => SecureSessionStore(),
);

final sessionControllerProvider =
    AsyncNotifierProvider<SessionController, SessionTokens?>(
      SessionController.new,
    );

abstract interface class SessionLifecycle {
  int get sessionGeneration;

  Future<bool> replaceAfterRefresh(
    SessionTokens tokens, {
    required int expectedGeneration,
  });

  Future<void> expireSession();
}

class SessionController extends AsyncNotifier<SessionTokens?>
    implements SessionLifecycle {
  static const _logoutRevokeTimeout = Duration(seconds: 3);

  int _sessionGeneration = 0;
  Future<void> _mutationTail = Future.value();

  @override
  int get sessionGeneration => _sessionGeneration;

  @override
  Future<SessionTokens?> build() => ref.watch(sessionStoreProvider).read();

  Future<void> save(SessionTokens tokens) async {
    final saveGeneration = ++_sessionGeneration;
    state = const AsyncLoading();
    try {
      final accepted = await _withSessionMutation(() async {
        if (saveGeneration != _sessionGeneration) {
          return false;
        }
        await ref.read(sessionStoreProvider).write(tokens);
        return saveGeneration == _sessionGeneration;
      });
      if (accepted) {
        state = AsyncData(tokens);
      }
    } on Object catch (error, stackTrace) {
      try {
        await _clearLocalSession();
      } on Object catch (clearError, clearStackTrace) {
        state = AsyncError(clearError, clearStackTrace);
        return;
      }
      state = AsyncError(error, stackTrace);
    }
  }

  @override
  Future<bool> replaceAfterRefresh(
    SessionTokens tokens, {
    required int expectedGeneration,
  }) {
    return _withSessionMutation(() async {
      if (expectedGeneration != _sessionGeneration) {
        return false;
      }
      await ref.read(sessionStoreProvider).write(tokens);
      if (expectedGeneration != _sessionGeneration) {
        return false;
      }
      state = AsyncData(tokens);
      return true;
    });
  }

  Future<void> signOut() async {
    final currentTokens = state.value;
    _sessionGeneration++;
    state = const AsyncLoading();
    state = await AsyncValue.guard(() async {
      await _revokeSessionBestEffort(currentTokens);
      await _clearLocalSession();
      return null;
    });
  }

  @override
  Future<void> expireSession() async {
    _sessionGeneration++;
    try {
      await _clearLocalSession();
      state = const AsyncData(null);
    } on Object catch (error, stackTrace) {
      state = AsyncError(error, stackTrace);
      rethrow;
    }
  }

  void retry() {
    ref.invalidateSelf();
  }

  Future<void> _revokeSessionBestEffort(SessionTokens? tokens) async {
    if (tokens == null) {
      return;
    }
    try {
      await ref
          .read(authRepositoryProvider)
          .revokeSession(tokens.refreshToken)
          .timeout(_logoutRevokeTimeout);
    } on Object {
      // A user-requested logout always erases local credentials. If the
      // backend is unreachable, this device can no longer retry with the
      // erased secret and the server-side family expires by policy.
    }
  }

  Future<void> _clearLocalSession() async {
    try {
      await ref.read(authRepositoryProvider).signOutFromProvider();
    } on Object {
      // Provider cleanup must not prevent local credentials from being erased.
    }
    await _withSessionMutation(() async {
      await Future.wait([
        ref.read(sessionStoreProvider).clear(),
        ref.read(careQueueStoreProvider).clear(),
      ]);
    });
  }

  Future<T> _withSessionMutation<T>(Future<T> Function() operation) {
    final completer = Completer<T>();
    _mutationTail = _mutationTail.then((_) async {
      try {
        completer.complete(await operation());
      } on Object catch (error, stackTrace) {
        completer.completeError(error, stackTrace);
      }
    });
    return completer.future;
  }
}
