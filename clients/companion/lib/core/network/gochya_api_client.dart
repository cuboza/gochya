import 'dart:async';
import 'dart:convert';

import 'package:http/http.dart' as http;

import '../models/auth_models.dart';
import '../models/care_models.dart';
import '../models/onboarding_models.dart';
import '../models/profile_models.dart';
import '../models/shop_models.dart';

class GochyaApiClient {
  GochyaApiClient({
    required Uri baseUri,
    http.Client? httpClient,
    this.requestTimeout = const Duration(seconds: 12),
  }) : baseUri = _validatedBaseUri(baseUri),
       _httpClient = httpClient ?? http.Client();

  final Uri baseUri;
  final Duration requestTimeout;
  final http.Client _httpClient;

  Future<AppleLoginChallenge> createAppleLoginChallenge() async {
    final json = await _postPublicObject('/v1/auth/apple/preflight');
    return _decode(
      'Apple login preflight',
      () => AppleLoginChallenge.fromJson(json),
    );
  }

  Future<AuthLoginResult> loginWithApple({
    required String identityToken,
    required String nonce,
    required String deviceId,
  }) async {
    if (identityToken.trim().isEmpty) {
      throw ArgumentError.value(
        identityToken,
        'identityToken',
        'must not be empty',
      );
    }
    if (nonce.trim().isEmpty) {
      throw ArgumentError.value(nonce, 'nonce', 'must not be empty');
    }
    if (deviceId.trim().isEmpty || deviceId.length > 128) {
      throw ArgumentError.value(
        deviceId,
        'deviceId',
        'must contain 1 to 128 characters',
      );
    }
    final json = await _postPublicObject(
      '/v1/auth/apple',
      body: {
        'identityToken': identityToken,
        'nonce': nonce,
        'deviceId': deviceId,
      },
    );
    return _decode('Apple login', () => AuthLoginResult.fromJson(json));
  }

  Future<AuthLoginResult> loginWithGoogle({
    required String idToken,
    required String deviceId,
  }) async {
    if (idToken.trim().isEmpty) {
      throw ArgumentError.value(idToken, 'idToken', 'must not be empty');
    }
    if (deviceId.trim().isEmpty || deviceId.length > 128) {
      throw ArgumentError.value(
        deviceId,
        'deviceId',
        'must contain 1 to 128 characters',
      );
    }
    final json = await _postPublicObject(
      '/v1/auth/google',
      body: {'idToken': idToken, 'deviceId': deviceId},
    );
    return _decode('Google login', () => AuthLoginResult.fromJson(json));
  }

  Future<AuthTokenPair> refreshSession(String refreshToken) async {
    if (refreshToken.trim().isEmpty) {
      throw ArgumentError.value(
        refreshToken,
        'refreshToken',
        'must not be empty',
      );
    }
    final json = await _postPublicObject(
      '/v1/auth/refresh',
      body: {'refreshToken': refreshToken},
    );
    return _decode('refresh session', () => AuthTokenPair.fromJson(json));
  }

  Future<void> logoutSession(String refreshToken) async {
    if (refreshToken.trim().isEmpty) {
      throw ArgumentError.value(
        refreshToken,
        'refreshToken',
        'must not be empty',
      );
    }
    final decoded = await _request(
      method: 'POST',
      path: '/v1/auth/logout',
      body: {'refreshToken': refreshToken},
    );
    if (decoded != null) {
      throw const ApiException(
        code: 'invalid_response',
        message: 'Server returned an invalid logout response.',
      );
    }
  }

  Future<PlayerProfile> getProfile(String accessToken) async {
    final json = await _getObject('/v1/me', accessToken);
    return _decode('profile', () => PlayerProfile.fromJson(json));
  }

  Future<ShopCatalog> getShopCatalog(String accessToken) async {
    final json = await _getObject('/v1/shop', accessToken);
    return _decode('shop catalog', () => ShopCatalog.fromJson(json));
  }

  Future<ShopInventory> getShopInventory(String accessToken) async {
    final json = await _getObject('/v1/me/items', accessToken);
    return _decode('shop inventory', () => ShopInventory.fromJson(json));
  }

  Future<ShopPurchase> purchaseShopItem({
    required String accessToken,
    required ShopItemId itemId,
    required int quantity,
    required String idempotencyKey,
  }) async {
    if (quantity < 1 || quantity > 100) {
      throw ArgumentError.value(
        quantity,
        'quantity',
        'must be between 1 and 100',
      );
    }
    final json = await _postObject(
      '/v1/shop/buy',
      accessToken,
      body: {'itemId': itemId.apiValue, 'quantity': quantity},
      idempotencyKey: idempotencyKey,
    );
    return _decode('shop purchase', () {
      final purchase = ShopPurchase.fromJson(json);
      if (purchase.itemId != itemId || purchase.purchasedQuantity != quantity) {
        throw const FormatException(
          'purchase response does not match the request',
        );
      }
      return purchase;
    });
  }

  Future<List<PetSummary>> getPets(String accessToken) async {
    final json = await _getArray('/v1/me/pets', accessToken);
    return _decode('pets', () {
      return json
          .map((value) => PetSummary.fromJson(asMap(value, 'pets[]')))
          .toList(growable: false);
    });
  }

  Future<LineageTree> getLineage(String accessToken, String petId) async {
    if (petId.trim().isEmpty) {
      throw ArgumentError.value(petId, 'petId', 'must not be empty');
    }
    final encodedPetId = Uri.encodeComponent(petId);
    final json = await _getObject(
      '/v1/me/pets/$encodedPetId/lineage',
      accessToken,
    );
    return _decode('lineage', () => LineageTree.fromJson(json));
  }

  Future<AgeGateResult> recordAgeGate({
    required String accessToken,
    required String birthDate,
    required String idempotencyKey,
  }) async {
    final json = await _postObject(
      '/v1/me/onboarding/age-gate',
      accessToken,
      body: {'birthDate': birthDate},
      idempotencyKey: idempotencyKey,
    );
    return _decode('age gate', () => AgeGateResult.fromJson(json));
  }

  Future<StarterEggResult> selectStarterEgg({
    required String accessToken,
    required StarterElement element,
    required String idempotencyKey,
  }) async {
    final json = await _postObject(
      '/v1/me/onboarding/starter-egg',
      accessToken,
      body: {'element': element.apiValue},
      idempotencyKey: idempotencyKey,
    );
    return _decode('starter egg', () => StarterEggResult.fromJson(json));
  }

  Future<List<EggSummary>> getEggs(String accessToken) async {
    final json = await _getArray('/v1/me/eggs', accessToken);
    return _decode('eggs', () {
      return json
          .map((value) => EggSummary.fromJson(asMap(value, 'eggs[]')))
          .toList(growable: false);
    });
  }

  Future<HatchedPet> hatchEgg(String accessToken, String eggId) async {
    if (eggId.trim().isEmpty) {
      throw ArgumentError.value(eggId, 'eggId', 'must not be empty');
    }
    final encodedEggId = Uri.encodeComponent(eggId);
    final json = await _postObject(
      '/v1/me/eggs/$encodedEggId/hatch',
      accessToken,
    );
    return _decode('hatched pet', () => HatchedPet.fromJson(json));
  }

  Future<CareSyncResult> reconcileCare({
    required String accessToken,
    required String deviceId,
    required String petId,
    required int baseRevision,
    required CareIntent intent,
  }) {
    return reconcileCareBatch(
      accessToken: accessToken,
      deviceId: deviceId,
      commands: [
        QueuedCareCommand(
          sequence: 1,
          petId: petId,
          baseRevision: baseRevision,
          intent: intent,
        ),
      ],
    );
  }

  Future<CareSyncResult> reconcileCareBatch({
    required String accessToken,
    required String deviceId,
    required List<QueuedCareCommand> commands,
  }) async {
    if (deviceId.trim().isEmpty || deviceId.length > 128) {
      throw ArgumentError.value(
        deviceId,
        'deviceId',
        'must contain 1 to 128 characters',
      );
    }
    if (commands.isEmpty || commands.length > 100) {
      throw ArgumentError.value(
        commands.length,
        'commands',
        'must contain between 1 and 100 entries',
      );
    }
    final petId = commands.first.petId;
    if (petId.trim().isEmpty ||
        commands.any((command) => command.petId != petId)) {
      throw ArgumentError('one care batch must target one non-empty petId');
    }
    if (commands.any(
      (command) =>
          command.baseRevision < 0 ||
          command.intent.clientMonotonicOffsetMs < 0,
    )) {
      throw ArgumentError('care revisions and offsets must not be negative');
    }
    final firstRevision = commands.first.baseRevision;
    final json = await _postObject(
      '/v1/sync/commands',
      accessToken,
      body: {
        'deviceId': deviceId,
        'commands': commands
            .map((command) => command.toApiJson())
            .toList(growable: false),
      },
      extraHeaders: {'If-Match': '$firstRevision'},
    );
    return _decode('care sync', () => CareSyncResult.fromJson(json));
  }

  void close() => _httpClient.close();

  Future<JsonMap> _getObject(String path, String accessToken) async {
    final decoded = await _get(path, accessToken);
    if (decoded is! Map<String, dynamic>) {
      throw const ApiException(
        code: 'invalid_response',
        message: 'Server returned an invalid object response.',
      );
    }
    return decoded;
  }

  Future<List<dynamic>> _getArray(String path, String accessToken) async {
    final decoded = await _get(path, accessToken);
    if (decoded is! List<dynamic>) {
      throw const ApiException(
        code: 'invalid_response',
        message: 'Server returned an invalid array response.',
      );
    }
    return decoded;
  }

  Future<JsonMap> _postObject(
    String path,
    String accessToken, {
    JsonMap? body,
    String? idempotencyKey,
    Map<String, String>? extraHeaders,
  }) async {
    final decoded = await _request(
      method: 'POST',
      path: path,
      accessToken: accessToken,
      body: body,
      idempotencyKey: idempotencyKey,
      extraHeaders: extraHeaders,
    );
    if (decoded is! Map<String, dynamic>) {
      throw const ApiException(
        code: 'invalid_response',
        message: 'Server returned an invalid object response.',
      );
    }
    return decoded;
  }

  Future<JsonMap> _postPublicObject(String path, {JsonMap? body}) async {
    final decoded = await _request(method: 'POST', path: path, body: body);
    if (decoded is! Map<String, dynamic>) {
      throw const ApiException(
        code: 'invalid_response',
        message: 'Server returned an invalid object response.',
      );
    }
    return decoded;
  }

  Future<Object?> _get(String path, String accessToken) async {
    return _request(method: 'GET', path: path, accessToken: accessToken);
  }

  Future<Object?> _request({
    required String method,
    required String path,
    String? accessToken,
    JsonMap? body,
    String? idempotencyKey,
    Map<String, String>? extraHeaders,
  }) async {
    if (accessToken != null && accessToken.trim().isEmpty) {
      throw ArgumentError.value(
        accessToken,
        'accessToken',
        'must not be empty',
      );
    }

    final headers = <String, String>{
      'Accept': 'application/json',
      if (accessToken != null) 'Authorization': 'Bearer $accessToken',
      if (body != null) 'Content-Type': 'application/json; charset=utf-8',
      'Idempotency-Key': ?idempotencyKey,
      ...?extraHeaders,
    };
    late final http.Response response;
    try {
      final uri = baseUri.resolve(path);
      final request = switch (method) {
        'GET' => _httpClient.get(uri, headers: headers),
        'POST' => _httpClient.post(
          uri,
          headers: headers,
          body: body == null ? null : jsonEncode(body),
        ),
        _ => throw StateError('unsupported HTTP method $method'),
      };
      response = await request.timeout(requestTimeout);
    } on TimeoutException {
      throw const ApiException(
        code: 'request_timeout',
        message: 'The server did not respond in time.',
      );
    } on http.ClientException {
      throw const ApiException(
        code: 'network_error',
        message: 'The server could not be reached.',
      );
    }

    if (response.statusCode == 204) {
      if (response.bodyBytes.isNotEmpty) {
        throw ApiException(
          statusCode: response.statusCode,
          code: 'invalid_response',
          message: 'Server returned a body with a no-content response.',
          requestId: response.headers['x-request-id'],
        );
      }
      return null;
    }

    Object? decoded;
    try {
      decoded = jsonDecode(utf8.decode(response.bodyBytes));
    } on FormatException {
      throw ApiException(
        statusCode: response.statusCode,
        code: 'invalid_response',
        message: 'Server returned malformed JSON.',
        requestId: response.headers['x-request-id'],
      );
    }

    if (response.statusCode < 200 || response.statusCode >= 300) {
      throw ApiException.fromResponse(
        statusCode: response.statusCode,
        decodedBody: decoded,
        headerRequestId: response.headers['x-request-id'],
      );
    }
    return decoded;
  }

  T _decode<T>(String resource, T Function() decode) {
    try {
      return decode();
    } on FormatException {
      throw ApiException(
        code: 'invalid_response',
        message: 'Server returned an invalid $resource payload.',
      );
    }
  }

  static Uri _validatedBaseUri(Uri value) {
    final isLoopbackHttp =
        value.scheme == 'http' &&
        (value.host == 'localhost' ||
            value.host == '127.0.0.1' ||
            value.host == '::1' ||
            value.host == '10.0.2.2' ||
            value.host == '10.0.3.2');
    if ((!value.hasScheme ||
            !value.hasAuthority ||
            (value.scheme != 'https' && !isLoopbackHttp)) ||
        (value.path.isNotEmpty && value.path != '/') ||
        value.hasQuery ||
        value.hasFragment ||
        value.userInfo.isNotEmpty) {
      throw ArgumentError.value(
        value,
        'baseUri',
        'must be an HTTPS origin (or loopback HTTP origin)',
      );
    }
    return value;
  }
}

class ApiException implements Exception {
  const ApiException({
    required this.code,
    required this.message,
    this.statusCode,
    this.requestId,
  });

  factory ApiException.fromResponse({
    required int statusCode,
    required Object? decodedBody,
    String? headerRequestId,
  }) {
    if (decodedBody case {
      'error': {
        'code': final String code,
        'message': final String message,
        'request_id': final String requestId,
      },
    }) {
      return ApiException(
        statusCode: statusCode,
        code: code,
        message: message,
        requestId: requestId,
      );
    }
    return ApiException(
      statusCode: statusCode,
      code: 'http_error',
      message: 'Server request failed with status $statusCode.',
      requestId: headerRequestId,
    );
  }

  final int? statusCode;
  final String code;
  final String message;
  final String? requestId;

  bool get isUnauthorized => statusCode == 401;

  @override
  String toString() => 'ApiException($code, status: $statusCode)';
}
