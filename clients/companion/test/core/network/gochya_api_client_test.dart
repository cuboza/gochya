import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:gochya_companion/core/network/gochya_api_client.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

void main() {
  group('GochyaApiClient', () {
    test('sends bearer auth and decodes profile and pets', () async {
      final seenPaths = <String>[];
      final client = GochyaApiClient(
        baseUri: Uri.parse('https://api.example.test'),
        httpClient: MockClient((request) async {
          expect(request.headers['authorization'], 'Bearer access-token');
          expect(request.headers['accept'], 'application/json');
          seenPaths.add(request.url.path);
          if (request.url.path == '/v1/me') {
            return _jsonResponse(_profileJson, 200);
          }
          if (request.url.path == '/v1/me/pets') {
            return _jsonResponse([_petJson], 200);
          }
          return http.Response('not found', 404);
        }),
      );

      final profile = await client.getProfile('access-token');
      final pets = await client.getPets('access-token');

      expect(profile.label, 'Ника');
      expect(profile.activePetId, 'pet-1');
      expect(pets, hasLength(1));
      expect(pets.single.needs.energy, 72);
      expect(seenPaths, ['/v1/me', '/v1/me/pets']);
    });

    test('decodes and validates bounded lineage', () async {
      final client = GochyaApiClient(
        baseUri: Uri.parse('https://api.example.test'),
        httpClient: MockClient((request) async {
          expect(request.url.path, '/v1/me/pets/pet-1/lineage');
          return _jsonResponse({
            'rootId': 'pet-1',
            'maxDepth': 3,
            'truncated': true,
            'nodes': [
              {
                'id': 'pet-1',
                'genome': {'element': 'Earth'},
                'name': 'Моти',
                'stage': 'baby',
                'level': 4,
                'generation': 1,
                'mutatedGenes': 5,
                'parentAId': 'pet-a',
                'parentBId': 'pet-b',
                'ancestorDepth': 0,
              },
              {
                'id': 'pet-a',
                'genome': {'element': 'Water'},
                'stage': 'adult',
                'level': 18,
                'generation': 0,
                'mutatedGenes': 0,
                'ancestorDepth': 1,
              },
            ],
          }, 200);
        }),
      );

      final lineage = await client.getLineage('access-token', 'pet-1');

      expect(lineage.rootId, 'pet-1');
      expect(lineage.truncated, isTrue);
      expect(lineage.nodes.last.ancestorDepth, 1);
      expect(lineage.nodes.first.mutatedGenes, 5);
    });

    test('preserves structured server errors', () async {
      final client = GochyaApiClient(
        baseUri: Uri.parse('https://api.example.test'),
        httpClient: MockClient((request) async {
          return _jsonResponse({
            'error': {
              'code': 'token_expired',
              'message': 'access token has expired',
              'details': <String, dynamic>{},
              'request_id': 'request-123',
            },
          }, 401);
        }),
      );

      await expectLater(
        client.getProfile('access-token'),
        throwsA(
          isA<ApiException>()
              .having((error) => error.statusCode, 'statusCode', 401)
              .having((error) => error.code, 'code', 'token_expired')
              .having((error) => error.requestId, 'requestId', 'request-123'),
        ),
      );
    });

    test('rejects malformed needs instead of clamping them', () async {
      final malformedPet = Map<String, dynamic>.from(_petJson);
      malformedPet['needs'] = {
        'hunger': 101,
        'energy': 72,
        'hygiene': 65,
        'mood': 94,
      };
      final client = GochyaApiClient(
        baseUri: Uri.parse('https://api.example.test'),
        httpClient: MockClient((request) async {
          return _jsonResponse([malformedPet], 200);
        }),
      );

      await expectLater(
        client.getPets('access-token'),
        throwsA(
          isA<ApiException>().having(
            (error) => error.code,
            'code',
            'invalid_response',
          ),
        ),
      );
    });

    test('allows plaintext only for loopback development', () {
      expect(
        () => GochyaApiClient(baseUri: Uri.parse('http://api.example.test')),
        throwsArgumentError,
      );
      expect(
        () => GochyaApiClient(
          baseUri: Uri.parse('https://api.example.test/prefix'),
        ),
        throwsArgumentError,
      );
      expect(
        () => GochyaApiClient(baseUri: Uri.parse('http://127.0.0.1:8080')),
        returnsNormally,
      );
      expect(
        () => GochyaApiClient(baseUri: Uri.parse('http://10.0.2.2:8080')),
        returnsNormally,
      );
    });
  });
}

final _profileJson = <String, dynamic>{
  'id': 'player-1',
  'username': 'nika',
  'displayName': 'Ника',
  'createdAt': '2026-07-24T10:00:00Z',
  'streakDays': 8,
  'activePetId': 'pet-1',
};

final _petJson = <String, dynamic>{
  'id': 'pet-1',
  'ownerId': 'player-1',
  'genome': {'element': 'Earth'},
  'name': 'Моти',
  'stage': 'baby',
  'level': 4,
  'xp': 320,
  'needs': {'hunger': 81, 'energy': 72, 'hygiene': 65, 'mood': 94},
  'stats': {'str': 2, 'agi': 3, 'end': 4, 'foc': 5},
  'generation': 1,
  'isActive': true,
  'createdAt': '2026-07-20T10:00:00Z',
  'parentAId': 'pet-a',
  'parentBId': 'pet-b',
  'isWeak': false,
  'careRevision': 9,
  'needsUpdatedAt': '2026-07-24T09:55:00Z',
};

http.Response _jsonResponse(Object? body, int statusCode) {
  return http.Response.bytes(
    utf8.encode(jsonEncode(body)),
    statusCode,
    headers: {'content-type': 'application/json; charset=utf-8'},
  );
}
