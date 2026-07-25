import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:gochya_companion/core/models/care_models.dart';
import 'package:gochya_companion/core/models/onboarding_models.dart';
import 'package:gochya_companion/core/models/shop_models.dart';
import 'package:gochya_companion/core/network/gochya_api_client.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

void main() {
  group('GochyaApiClient', () {
    test('performs Apple nonce preflight and token exchange', () async {
      final requests = <http.Request>[];
      final client = GochyaApiClient(
        baseUri: Uri.parse('https://api.example.test'),
        httpClient: MockClient((request) async {
          requests.add(request);
          if (request.url.path.endsWith('/preflight')) {
            return _jsonResponse({
              'nonce': 'server-apple-nonce',
              'expiresAt': '2026-07-25T10:05:00Z',
            }, 200);
          }
          return _jsonResponse({
            'jwt': 'apple-access',
            'refreshToken': 'apple-refresh',
            'accessTokenExpiresAt': '2026-07-25T10:15:00Z',
            'refreshTokenExpiresAt': '2026-08-24T10:00:00Z',
            'player': {'id': 'player-1', 'username': 'apple-player'},
          }, 200);
        }),
      );

      final challenge = await client.createAppleLoginChallenge();
      final result = await client.loginWithApple(
        identityToken: 'apple-identity-token',
        nonce: challenge.nonce,
        deviceId: '11111111-1111-4111-8111-111111111111',
      );

      expect(requests, hasLength(2));
      expect(requests.first.method, 'POST');
      expect(requests.first.url.path, '/v1/auth/apple/preflight');
      expect(requests.first.body, isEmpty);
      expect(requests.first.headers, isNot(contains('authorization')));
      expect(jsonDecode(requests.last.body), {
        'identityToken': 'apple-identity-token',
        'nonce': 'server-apple-nonce',
        'deviceId': '11111111-1111-4111-8111-111111111111',
      });
      expect(challenge.expiresAt, DateTime.parse('2026-07-25T10:05:00Z'));
      expect(result.tokens.accessToken, 'apple-access');
      expect(result.player.username, 'apple-player');
    });

    test('exchanges a Google ID token without bearer credentials', () async {
      late http.Request request;
      final client = GochyaApiClient(
        baseUri: Uri.parse('https://api.example.test'),
        httpClient: MockClient((value) async {
          request = value;
          return _jsonResponse({
            'jwt': 'issued-access',
            'refreshToken': 'issued-refresh',
            'accessTokenExpiresAt': '2026-07-25T10:15:00Z',
            'refreshTokenExpiresAt': '2026-08-24T10:00:00Z',
            'player': {
              'id': 'player-1',
              'username': 'nika',
              'displayName': 'Ника',
            },
          }, 200);
        }),
      );

      final result = await client.loginWithGoogle(
        idToken: 'verified-google-id-token',
        deviceId: '11111111-1111-4111-8111-111111111111',
      );

      expect(request.method, 'POST');
      expect(request.url.path, '/v1/auth/google');
      expect(request.headers, isNot(contains('authorization')));
      expect(jsonDecode(request.body), {
        'idToken': 'verified-google-id-token',
        'deviceId': '11111111-1111-4111-8111-111111111111',
      });
      expect(result.tokens.accessToken, 'issued-access');
      expect(result.tokens.refreshToken, 'issued-refresh');
      expect(result.player.id, 'player-1');
      expect(result.player.displayName, 'Ника');
    });

    test('rotates refresh token without sending bearer credentials', () async {
      late http.Request request;
      final client = GochyaApiClient(
        baseUri: Uri.parse('https://api.example.test'),
        httpClient: MockClient((value) async {
          request = value;
          return _jsonResponse({
            'jwt': 'rotated-access',
            'refreshToken': 'rotated-refresh',
            'accessTokenExpiresAt': '2026-07-25T10:15:00Z',
            'refreshTokenExpiresAt': '2026-08-24T10:00:00Z',
          }, 200);
        }),
      );

      final pair = await client.refreshSession('current-refresh');

      expect(request.method, 'POST');
      expect(request.url.path, '/v1/auth/refresh');
      expect(request.headers, isNot(contains('authorization')));
      expect(jsonDecode(request.body), {'refreshToken': 'current-refresh'});
      expect(pair.accessToken, 'rotated-access');
      expect(pair.refreshToken, 'rotated-refresh');
      expect(pair.accessTokenExpiresAt, DateTime.parse('2026-07-25T10:15:00Z'));
    });

    test('revokes a refresh family and accepts no-content response', () async {
      late http.Request request;
      final client = GochyaApiClient(
        baseUri: Uri.parse('https://api.example.test'),
        httpClient: MockClient((value) async {
          request = value;
          return http.Response('', 204);
        }),
      );

      await client.logoutSession('current-refresh');

      expect(request.method, 'POST');
      expect(request.url.path, '/v1/auth/logout');
      expect(request.headers, isNot(contains('authorization')));
      expect(jsonDecode(request.body), {'refreshToken': 'current-refresh'});
    });

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

    test('loads the shop and buys only by item, quantity, and key', () async {
      final requests = <http.Request>[];
      final client = GochyaApiClient(
        baseUri: Uri.parse('https://api.example.test'),
        httpClient: MockClient((request) async {
          requests.add(request);
          expect(request.headers['authorization'], 'Bearer access-token');
          return switch (request.url.path) {
            '/v1/shop' => _jsonResponse({
              'items': [
                {
                  'id': 'apple',
                  'category': 'care',
                  'currency': 'koins',
                  'unitPrice': 20,
                  'isStackable': true,
                },
                {
                  'id': 'love_crystal',
                  'category': 'breeding',
                  'currency': 'koins',
                  'unitPrice': 200,
                  'isStackable': true,
                },
              ],
            }, 200),
            '/v1/me/items' => _jsonResponse({
              'koins': 100,
              'items': [
                {'itemId': 'apple', 'quantity': 2},
              ],
            }, 200),
            '/v1/shop/buy' => _jsonResponse({
              'itemId': 'apple',
              'purchasedQuantity': 1,
              'itemQuantity': 3,
              'unitPriceKoins': 20,
              'koinsSpent': 20,
              'koinsRemaining': 80,
              'purchasedAt': '2026-07-25T12:00:00Z',
            }, 200),
            _ => http.Response('not found', 404),
          };
        }),
      );

      final catalog = await client.getShopCatalog('access-token');
      final inventory = await client.getShopInventory('access-token');
      final purchase = await client.purchaseShopItem(
        accessToken: 'access-token',
        itemId: ShopItemId.apple,
        quantity: 1,
        idempotencyKey: '11111111-1111-4111-8111-111111111111',
      );

      expect(catalog.items, hasLength(2));
      expect(catalog.items.first.unitPrice, 20);
      expect(inventory.koins, 100);
      expect(inventory.quantityOf(ShopItemId.apple), 2);
      expect(purchase.koinsRemaining, 80);
      expect(purchase.itemQuantity, 3);
      expect(requests.last.method, 'POST');
      expect(requests.last.headers['idempotency-key'], isNotNull);
      expect(
        requests.last.headers['idempotency-key'],
        '11111111-1111-4111-8111-111111111111',
      );
      expect(jsonDecode(requests.last.body), {
        'itemId': 'apple',
        'quantity': 1,
      });
    });

    test('rejects inconsistent authoritative shop payloads', () async {
      var call = 0;
      final client = GochyaApiClient(
        baseUri: Uri.parse('https://api.example.test'),
        httpClient: MockClient((request) async {
          call++;
          if (call == 1) {
            return _jsonResponse({
              'items': [
                {
                  'id': 'apple',
                  'category': 'breeding',
                  'currency': 'koins',
                  'unitPrice': 20,
                  'isStackable': true,
                },
              ],
            }, 200);
          }
          return _jsonResponse({
            'itemId': 'steak',
            'purchasedQuantity': 2,
            'itemQuantity': 2,
            'unitPriceKoins': 20,
            'koinsSpent': 40,
            'koinsRemaining': 80,
            'purchasedAt': '2026-07-25T12:00:00Z',
          }, 200);
        }),
      );

      await expectLater(
        client.getShopCatalog('access-token'),
        throwsA(
          isA<ApiException>().having(
            (error) => error.code,
            'code',
            'invalid_response',
          ),
        ),
      );
      await expectLater(
        client.purchaseShopItem(
          accessToken: 'access-token',
          itemId: ShopItemId.apple,
          quantity: 2,
          idempotencyKey: '11111111-1111-4111-8111-111111111111',
        ),
        throwsA(
          isA<ApiException>().having(
            (error) => error.code,
            'code',
            'invalid_response',
          ),
        ),
      );
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

    test(
      'sends idempotent onboarding requests and decodes responses',
      () async {
        final requests = <http.Request>[];
        final client = GochyaApiClient(
          baseUri: Uri.parse('https://api.example.test'),
          httpClient: MockClient((request) async {
            requests.add(request);
            if (request.url.path.endsWith('/age-gate')) {
              return _jsonResponse({
                'status': 'eligible',
                'coppaRestricted': false,
                'recordedAt': '2026-07-25T10:00:00Z',
              }, 200);
            }
            return _jsonResponse({
              'eggId': 'egg-1',
              'element': 'water',
              'incubateUntil': '2026-07-25T10:00:05Z',
            }, 200);
          }),
        );

        final age = await client.recordAgeGate(
          accessToken: 'access-token',
          birthDate: '2000-01-02',
          idempotencyKey: '11111111-1111-4111-8111-111111111111',
        );
        final starter = await client.selectStarterEgg(
          accessToken: 'access-token',
          element: StarterElement.water,
          idempotencyKey: '22222222-2222-4222-8222-222222222222',
        );

        expect(age.isEligible, isTrue);
        expect(starter.element, StarterElement.water);
        expect(requests, hasLength(2));
        expect(requests.first.method, 'POST');
        expect(
          requests.first.headers['idempotency-key'],
          '11111111-1111-4111-8111-111111111111',
        );
        expect(jsonDecode(requests.first.body), {'birthDate': '2000-01-02'});
        expect(
          requests.last.headers['idempotency-key'],
          '22222222-2222-4222-8222-222222222222',
        );
        expect(jsonDecode(requests.last.body), {'element': 'water'});
      },
    );

    test('lists an incubating egg and hatches with an empty body', () async {
      final requests = <http.Request>[];
      final client = GochyaApiClient(
        baseUri: Uri.parse('https://api.example.test'),
        httpClient: MockClient((request) async {
          requests.add(request);
          if (request.method == 'GET') {
            return _jsonResponse([
              {
                'id': 'egg-1',
                'ownerId': 'player-1',
                'origin': 'starter',
                'genome': {'element': 0},
                'incubateUntil': '2026-07-25T10:00:05Z',
                'mutatedGenes': 0,
                'createdAt': '2026-07-25T10:00:00Z',
              },
            ], 200);
          }
          return _jsonResponse({
            'id': 'pet-1',
            'ownerId': 'player-1',
            'genome': {'element': 0},
            'stage': 'baby',
            'level': 1,
            'xp': 0,
            'needs': {
              'hunger': 100,
              'energy': 100,
              'hygiene': 100,
              'mood': 100,
            },
            'stats': {'str': 1, 'agi': 1, 'end': 1, 'foc': 1},
            'generation': 0,
            'isActive': true,
            'createdAt': '2026-07-25T10:00:05Z',
            'isWeak': false,
          }, 200);
        }),
      );

      final eggs = await client.getEggs('access-token');
      final pet = await client.hatchEgg('access-token', 'egg-1');

      expect(eggs.single.origin, 'starter');
      expect(eggs.single.parentAId, isNull);
      expect(pet.id, 'pet-1');
      expect(pet.isActive, isTrue);
      expect(requests.last.method, 'POST');
      expect(requests.last.url.path, '/v1/me/eggs/egg-1/hatch');
      expect(requests.last.body, isEmpty);
    });

    test('sends revision-bound care command and decodes snapshot', () async {
      late http.Request request;
      final client = GochyaApiClient(
        baseUri: Uri.parse('https://api.example.test'),
        httpClient: MockClient((value) async {
          request = value;
          return _jsonResponse({
            'results': [
              {
                'operationId': '11111111-1111-4111-8111-111111111111',
                'status': 'APPLIED',
                'snapshot': _careSnapshotJson,
              },
            ],
            'canonicalSnapshots': [_careSnapshotJson],
            'newRevision': 10,
            'serverTime': '2026-07-25T10:00:01Z',
          }, 200);
        }),
      );
      final intent = CareIntent(
        operationId: '11111111-1111-4111-8111-111111111111',
        operation: CareOperation.feed,
        itemId: 'apple',
        clientWallTime: DateTime.parse('2026-07-25T10:00:00Z'),
        clientMonotonicOffsetMs: 1234,
      );

      final response = await client.reconcileCare(
        accessToken: 'access-token',
        deviceId: '22222222-2222-4222-8222-222222222222',
        petId: '33333333-3333-4333-8333-333333333333',
        baseRevision: 9,
        intent: intent,
      );

      expect(request.method, 'POST');
      expect(request.url.path, '/v1/sync/commands');
      expect(request.headers['if-match'], '9');
      final body = jsonDecode(request.body) as Map<String, dynamic>;
      expect(body['deviceId'], '22222222-2222-4222-8222-222222222222');
      final command =
          (body['commands'] as List<dynamic>).single as Map<String, dynamic>;
      expect(command['operationId'], intent.operationId);
      expect(command['aggregateType'], 'pet');
      expect(command['aggregateId'], '33333333-3333-4333-8333-333333333333');
      expect(command['baseRevision'], 9);
      expect(command['operationType'], 'feed');
      expect(command['arguments'], {'itemId': 'apple'});
      expect(command['clientMonotonicOffsetMs'], 1234);
      expect(command['schemaVersion'], 1);
      expect(response.resultFor(intent.operationId).status.isApplied, isTrue);
      expect(response.canonicalSnapshots.single.revision, 10);
    });

    test('rejects inconsistent age and egg payloads', () async {
      var call = 0;
      final client = GochyaApiClient(
        baseUri: Uri.parse('https://api.example.test'),
        httpClient: MockClient((request) async {
          call++;
          if (call == 1) {
            return _jsonResponse({
              'status': 'eligible',
              'coppaRestricted': true,
              'recordedAt': '2026-07-25T10:00:00Z',
            }, 200);
          }
          return _jsonResponse([
            {
              'id': 'egg-1',
              'ownerId': 'player-1',
              'origin': 'starter',
              'genome': {'element': 0},
              'parentAId': 'pet-a',
              'parentBId': 'pet-b',
              'incubateUntil': '2026-07-25T10:00:05Z',
              'mutatedGenes': 0,
              'createdAt': '2026-07-25T10:00:00Z',
            },
          ], 200);
        }),
      );

      await expectLater(
        client.recordAgeGate(
          accessToken: 'access-token',
          birthDate: '2000-01-02',
          idempotencyKey: '11111111-1111-4111-8111-111111111111',
        ),
        throwsA(
          isA<ApiException>().having(
            (error) => error.code,
            'code',
            'invalid_response',
          ),
        ),
      );
      await expectLater(
        client.getEggs('access-token'),
        throwsA(
          isA<ApiException>().having(
            (error) => error.code,
            'code',
            'invalid_response',
          ),
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

final _careSnapshotJson = <String, dynamic>{
  'id': '33333333-3333-4333-8333-333333333333',
  'needs': {'hunger': 91, 'energy': 72, 'hygiene': 65, 'mood': 94},
  'revision': 10,
  'isWeak': false,
  'needsUpdatedAt': '2026-07-25T10:00:01Z',
};

http.Response _jsonResponse(Object? body, int statusCode) {
  return http.Response.bytes(
    utf8.encode(jsonEncode(body)),
    statusCode,
    headers: {'content-type': 'application/json; charset=utf-8'},
  );
}
