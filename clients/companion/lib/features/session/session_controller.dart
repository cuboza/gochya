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
  Future<void> replaceAfterRefresh(SessionTokens tokens);

  Future<void> expireSession();
}

class SessionController extends AsyncNotifier<SessionTokens?>
    implements SessionLifecycle {
  @override
  Future<SessionTokens?> build() => ref.watch(sessionStoreProvider).read();

  Future<void> save(SessionTokens tokens) async {
    state = const AsyncLoading();
    try {
      await ref.read(sessionStoreProvider).write(tokens);
      state = AsyncData(tokens);
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
  Future<void> replaceAfterRefresh(SessionTokens tokens) async {
    await ref.read(sessionStoreProvider).write(tokens);
    state = AsyncData(tokens);
  }

  Future<void> signOut() async {
    state = const AsyncLoading();
    state = await AsyncValue.guard(() async {
      await _clearLocalSession();
      return null;
    });
  }

  @override
  Future<void> expireSession() async {
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

  Future<void> _clearLocalSession() async {
    try {
      await ref.read(authRepositoryProvider).signOutFromProvider();
    } on Object {
      // Provider cleanup must not prevent local credentials from being erased.
    }
    await Future.wait([
      ref.read(sessionStoreProvider).clear(),
      ref.read(careQueueStoreProvider).clear(),
    ]);
  }
}
