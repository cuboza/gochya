import 'package:flutter/foundation.dart';
import 'package:google_sign_in/google_sign_in.dart';

enum GoogleIdentityFailure {
  cancelled,
  configuration,
  unavailable,
  missingIdToken,
}

class GoogleIdentityException implements Exception {
  const GoogleIdentityException(this.failure);

  final GoogleIdentityFailure failure;

  @override
  String toString() => 'GoogleIdentityException($failure)';
}

abstract interface class GoogleIdentityClient {
  bool get isAvailable;

  Future<String> authenticateIdToken();

  Future<void> signOut();
}

class NativeGoogleIdentityClient implements GoogleIdentityClient {
  NativeGoogleIdentityClient({
    required String serverClientId,
    GoogleSignIn? googleSignIn,
    bool? isAndroid,
  }) : _serverClientId = serverClientId.trim(),
       _googleSignIn = googleSignIn ?? GoogleSignIn.instance,
       _isAndroid =
           isAndroid ??
           (!kIsWeb && defaultTargetPlatform == TargetPlatform.android);

  final String _serverClientId;
  final GoogleSignIn _googleSignIn;
  final bool _isAndroid;
  Future<void>? _initialization;

  @override
  bool get isAvailable => _isAndroid && _serverClientId.isNotEmpty;

  @override
  Future<String> authenticateIdToken() async {
    if (!isAvailable) {
      throw const GoogleIdentityException(GoogleIdentityFailure.configuration);
    }
    try {
      await _ensureInitialized();
      if (!_googleSignIn.supportsAuthenticate()) {
        throw const GoogleIdentityException(GoogleIdentityFailure.unavailable);
      }
      final account = await _googleSignIn.authenticate();
      final idToken = account.authentication.idToken?.trim();
      if (idToken == null || idToken.isEmpty) {
        await _bestEffortSignOut();
        throw const GoogleIdentityException(
          GoogleIdentityFailure.missingIdToken,
        );
      }
      return idToken;
    } on GoogleIdentityException {
      rethrow;
    } on GoogleSignInException catch (error) {
      throw GoogleIdentityException(_mapFailure(error.code));
    }
  }

  @override
  Future<void> signOut() async {
    if (_initialization == null) {
      return;
    }
    await _googleSignIn.signOut();
  }

  Future<void> _bestEffortSignOut() async {
    try {
      await _googleSignIn.signOut();
    } on Object {
      // Missing identity proof remains the primary failure.
    }
  }

  Future<void> _ensureInitialized() {
    final existing = _initialization;
    if (existing != null) {
      return existing;
    }
    final started = _initialize();
    _initialization = started;
    return started;
  }

  Future<void> _initialize() async {
    try {
      await _googleSignIn.initialize(serverClientId: _serverClientId);
    } on Object {
      _initialization = null;
      rethrow;
    }
  }

  static GoogleIdentityFailure _mapFailure(GoogleSignInExceptionCode code) {
    return switch (code) {
      GoogleSignInExceptionCode.canceled => GoogleIdentityFailure.cancelled,
      GoogleSignInExceptionCode.clientConfigurationError ||
      GoogleSignInExceptionCode.providerConfigurationError =>
        GoogleIdentityFailure.configuration,
      _ => GoogleIdentityFailure.unavailable,
    };
  }
}
