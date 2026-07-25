import 'profile_models.dart';

class AppleLoginChallenge {
  const AppleLoginChallenge({required this.nonce, required this.expiresAt});

  factory AppleLoginChallenge.fromJson(JsonMap json) {
    return AppleLoginChallenge(
      nonce: requiredString(json, 'nonce'),
      expiresAt: requiredDateTime(json, 'expiresAt'),
    );
  }

  final String nonce;
  final DateTime expiresAt;
}

class AuthTokenPair {
  const AuthTokenPair({
    required this.accessToken,
    required this.refreshToken,
    required this.accessTokenExpiresAt,
    required this.refreshTokenExpiresAt,
  });

  factory AuthTokenPair.fromJson(JsonMap json) {
    final accessTokenExpiresAt = requiredDateTime(json, 'accessTokenExpiresAt');
    final refreshTokenExpiresAt = requiredDateTime(
      json,
      'refreshTokenExpiresAt',
    );
    if (!refreshTokenExpiresAt.isAfter(accessTokenExpiresAt)) {
      throw const FormatException(
        'refresh token must outlive the access token',
      );
    }
    return AuthTokenPair(
      accessToken: requiredString(json, 'jwt'),
      refreshToken: requiredString(json, 'refreshToken'),
      accessTokenExpiresAt: accessTokenExpiresAt,
      refreshTokenExpiresAt: refreshTokenExpiresAt,
    );
  }

  final String accessToken;
  final String refreshToken;
  final DateTime accessTokenExpiresAt;
  final DateTime refreshTokenExpiresAt;
}

class AuthPlayer {
  const AuthPlayer({
    required this.id,
    required this.username,
    this.displayName,
  });

  factory AuthPlayer.fromJson(JsonMap json) {
    return AuthPlayer(
      id: requiredString(json, 'id'),
      username: requiredString(json, 'username'),
      displayName: optionalString(json, 'displayName'),
    );
  }

  final String id;
  final String username;
  final String? displayName;
}

class AuthLoginResult {
  const AuthLoginResult({required this.tokens, required this.player});

  factory AuthLoginResult.fromJson(JsonMap json) {
    return AuthLoginResult(
      tokens: AuthTokenPair.fromJson(json),
      player: AuthPlayer.fromJson(requiredMap(json, 'player')),
    );
  }

  final AuthTokenPair tokens;
  final AuthPlayer player;
}
