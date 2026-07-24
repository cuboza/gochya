import 'dart:async';
import 'dart:convert';

import 'package:http/http.dart' as http;

import '../models/profile_models.dart';

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

  Future<PlayerProfile> getProfile(String accessToken) async {
    final json = await _getObject('/v1/me', accessToken);
    return _decode('profile', () => PlayerProfile.fromJson(json));
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

  Future<Object?> _get(String path, String accessToken) async {
    if (accessToken.trim().isEmpty) {
      throw ArgumentError.value(
        accessToken,
        'accessToken',
        'must not be empty',
      );
    }

    late final http.Response response;
    try {
      response = await _httpClient
          .get(
            baseUri.resolve(path),
            headers: {
              'Accept': 'application/json',
              'Authorization': 'Bearer $accessToken',
            },
          )
          .timeout(requestTimeout);
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
            value.host == '::1');
    if ((!value.hasScheme ||
            !value.hasAuthority ||
            (value.scheme != 'https' && !isLoopbackHttp)) ||
        value.hasQuery ||
        value.hasFragment ||
        value.userInfo.isNotEmpty) {
      throw ArgumentError.value(
        value,
        'baseUri',
        'must be HTTPS (or loopback HTTP) without query or fragment',
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
