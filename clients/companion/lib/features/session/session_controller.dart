import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/session/session_store.dart';

final sessionStoreProvider = Provider<SessionStore>(
  (ref) => SecureSessionStore(),
);

final sessionControllerProvider =
    AsyncNotifierProvider<SessionController, SessionTokens?>(
      SessionController.new,
    );

class SessionController extends AsyncNotifier<SessionTokens?> {
  @override
  Future<SessionTokens?> build() => ref.watch(sessionStoreProvider).read();

  Future<void> save(SessionTokens tokens) async {
    state = const AsyncLoading();
    state = await AsyncValue.guard(() async {
      await ref.read(sessionStoreProvider).write(tokens);
      return tokens;
    });
  }

  Future<void> signOut() async {
    state = const AsyncLoading();
    state = await AsyncValue.guard(() async {
      await ref.read(sessionStoreProvider).clear();
      return null;
    });
  }

  void retry() {
    ref.invalidateSelf();
  }
}
