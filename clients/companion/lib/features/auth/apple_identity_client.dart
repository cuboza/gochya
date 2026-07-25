import 'package:flutter/foundation.dart';
import 'package:sign_in_with_apple/sign_in_with_apple.dart';

enum AppleIdentityFailure { cancelled, unavailable, missingIdentityToken }

class AppleIdentityException implements Exception {
  const AppleIdentityException(this.failure);

  final AppleIdentityFailure failure;

  @override
  String toString() => 'AppleIdentityException($failure)';
}

abstract interface class AppleIdentityClient {
  Future<bool> isAvailable();

  Future<String> authenticateIdentityToken(String nonce);
}

class NativeAppleIdentityClient implements AppleIdentityClient {
  NativeAppleIdentityClient({bool? isIOS})
    : _isIOS =
          isIOS ?? (!kIsWeb && defaultTargetPlatform == TargetPlatform.iOS);

  final bool _isIOS;

  @override
  Future<bool> isAvailable() async {
    if (!_isIOS) {
      return false;
    }
    try {
      return await SignInWithApple.isAvailable();
    } on Object {
      return false;
    }
  }

  @override
  Future<String> authenticateIdentityToken(String nonce) async {
    if (nonce.trim().isEmpty) {
      throw ArgumentError.value(nonce, 'nonce', 'must not be empty');
    }
    if (!await isAvailable()) {
      throw const AppleIdentityException(AppleIdentityFailure.unavailable);
    }
    try {
      final credential = await SignInWithApple.getAppleIDCredential(
        scopes: const [],
        nonce: nonce,
      );
      final identityToken = credential.identityToken?.trim();
      if (identityToken == null || identityToken.isEmpty) {
        throw const AppleIdentityException(
          AppleIdentityFailure.missingIdentityToken,
        );
      }
      return identityToken;
    } on AppleIdentityException {
      rethrow;
    } on SignInWithAppleAuthorizationException catch (error) {
      if (error.code == AuthorizationErrorCode.canceled) {
        throw const AppleIdentityException(AppleIdentityFailure.cancelled);
      }
      throw const AppleIdentityException(AppleIdentityFailure.unavailable);
    } on SignInWithAppleException {
      throw const AppleIdentityException(AppleIdentityFailure.unavailable);
    }
  }
}
