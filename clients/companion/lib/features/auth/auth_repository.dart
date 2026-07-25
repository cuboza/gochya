import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/device/installation_id_store.dart';
import '../../core/network/api_providers.dart';
import '../../core/network/gochya_api_client.dart';
import '../../core/session/session_store.dart';
import 'apple_identity_client.dart';
import 'google_identity_client.dart';

final appleIdentityClientProvider = Provider<AppleIdentityClient>(
  (ref) => NativeAppleIdentityClient(),
);

final googleIdentityClientProvider = Provider<GoogleIdentityClient>((ref) {
  return NativeGoogleIdentityClient(
    serverClientId: ref.watch(appConfigProvider).googleServerClientId,
  );
});

final authRepositoryProvider = Provider<AuthRepository>((ref) {
  return ApiAuthRepository(
    apiClient: ref.watch(apiClientProvider),
    appleIdentityClient: ref.watch(appleIdentityClientProvider),
    identityClient: ref.watch(googleIdentityClientProvider),
    installationIdStore: ref.watch(installationIdStoreProvider),
  );
});

final appleSignInAvailabilityProvider = FutureProvider<bool>(
  (ref) => ref.watch(authRepositoryProvider).isAppleSignInAvailable(),
);

abstract interface class AuthRepository {
  bool get isGoogleSignInAvailable;

  Future<bool> isAppleSignInAvailable();

  Future<SessionTokens> signInWithApple();

  Future<SessionTokens> signInWithGoogle();

  Future<void> signOutFromProvider();
}

class ApiAuthRepository implements AuthRepository {
  const ApiAuthRepository({
    required this.apiClient,
    required this.appleIdentityClient,
    required this.identityClient,
    required this.installationIdStore,
  });

  final GochyaApiClient apiClient;
  final AppleIdentityClient appleIdentityClient;
  final GoogleIdentityClient identityClient;
  final InstallationIdStore installationIdStore;

  @override
  Future<bool> isAppleSignInAvailable() {
    return appleIdentityClient.isAvailable();
  }

  @override
  bool get isGoogleSignInAvailable => identityClient.isAvailable;

  @override
  Future<SessionTokens> signInWithApple() async {
    if (!await appleIdentityClient.isAvailable()) {
      throw const AppleIdentityException(AppleIdentityFailure.unavailable);
    }
    final challenge = await apiClient.createAppleLoginChallenge();
    final identityToken = await appleIdentityClient.authenticateIdentityToken(
      challenge.nonce,
    );
    final deviceId = await installationIdStore.getOrCreate();
    final result = await apiClient.loginWithApple(
      identityToken: identityToken,
      nonce: challenge.nonce,
      deviceId: deviceId,
    );
    return SessionTokens.fromAuthTokenPair(result.tokens);
  }

  @override
  Future<SessionTokens> signInWithGoogle() async {
    final idToken = await identityClient.authenticateIdToken();
    try {
      final deviceId = await installationIdStore.getOrCreate();
      final result = await apiClient.loginWithGoogle(
        idToken: idToken,
        deviceId: deviceId,
      );
      return SessionTokens.fromAuthTokenPair(result.tokens);
    } on Object {
      try {
        await identityClient.signOut();
      } on Object {
        // The exchange failure is the actionable error. The app still has no
        // GOCHYA session even if provider cleanup also fails.
      }
      rethrow;
    }
  }

  @override
  Future<void> signOutFromProvider() => identityClient.signOut();
}
