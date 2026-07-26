import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:gochya_companion/core/models/battle_models.dart';
import 'package:gochya_companion/core/models/breeding_models.dart';
import 'package:gochya_companion/core/models/care_models.dart';
import 'package:gochya_companion/core/models/onboarding_models.dart';
import 'package:gochya_companion/core/models/shop_models.dart';
import 'package:gochya_companion/core/models/technique_models.dart';
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
                'genome': {'element': 2},
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
                'genome': {'element': 1},
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

    test('follows technique cursors and rejects a duplicated card', () async {
      final requests = <http.Request>[];
      final client = GochyaApiClient(
        baseUri: Uri.parse('https://api.example.test'),
        httpClient: MockClient((request) async {
          requests.add(request);
          if (request.url.queryParameters['cursor'] == null) {
            return _jsonResponse({
              'items': [_techniqueJson],
              'next_cursor': 'opaque-cursor',
            }, 200);
          }
          return _jsonResponse({'items': <Object>[]}, 200);
        }),
      );

      final first = await client.getTechniques('access', limit: 50);
      final second = await client.getTechniques(
        'access',
        cursor: first.nextCursor,
      );

      expect(requests.first.url.path, '/v1/me/techniques');
      expect(requests.first.url.queryParameters, {'limit': '50'});
      expect(requests.last.url.queryParameters, {'cursor': 'opaque-cursor'});
      expect(first.items.single.type, TechniqueType.hook);
      expect(first.items.single.rarity, TechniqueRarity.rare);
      expect(first.nextCursor, 'opaque-cursor');
      expect(second.items, isEmpty);
      expect(second.nextCursor, isNull);
    });

    test('equips five cards and verifies the echoed loadout', () async {
      late http.Request request;
      final client = GochyaApiClient(
        baseUri: Uri.parse('https://api.example.test'),
        httpClient: MockClient((value) async {
          request = value;
          return _jsonResponse({
            'petId': 'pet-1',
            'cardIds': _cardIds,
            'signatureIdx': 2,
            'revision': 7,
            'updatedAt': '2026-07-25T10:00:00Z',
          }, 200);
        }),
      );

      final loadout = await client.equipTechniques(
        accessToken: 'access',
        cardIds: _cardIds,
        signatureIdx: 2,
        idempotencyKey: '11111111-1111-4111-8111-111111111111',
      );

      expect(request.method, 'POST');
      expect(request.url.path, '/v1/me/techniques/equip');
      expect(
        request.headers['idempotency-key'],
        '11111111-1111-4111-8111-111111111111',
      );
      expect(jsonDecode(request.body), {
        'cardIds': _cardIds,
        'signatureIdx': 2,
      });
      expect(loadout.revision, 7);
      expect(loadout.signatureCardId, _cardIds[2]);
    });

    test('rejects a loadout response that reorders the request', () async {
      final client = GochyaApiClient(
        baseUri: Uri.parse('https://api.example.test'),
        httpClient: MockClient((value) async {
          return _jsonResponse({
            'petId': 'pet-1',
            'cardIds': _cardIds.reversed.toList(),
            'signatureIdx': 2,
            'revision': 7,
            'updatedAt': '2026-07-25T10:00:00Z',
          }, 200);
        }),
      );

      await expectLater(
        client.equipTechniques(
          accessToken: 'access',
          cardIds: _cardIds,
          signatureIdx: 2,
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

    test('refuses to equip a set that is not five distinct cards', () {
      final client = GochyaApiClient(
        baseUri: Uri.parse('https://api.example.test'),
      );
      expect(
        () => client.equipTechniques(
          accessToken: 'access',
          cardIds: [..._cardIds.take(4), _cardIds.first],
          signatureIdx: 0,
          idempotencyKey: '11111111-1111-4111-8111-111111111111',
        ),
        throwsArgumentError,
      );
      expect(
        () => client.equipTechniques(
          accessToken: 'access',
          cardIds: _cardIds,
          signatureIdx: 5,
          idempotencyKey: '11111111-1111-4111-8111-111111111111',
        ),
        throwsArgumentError,
      );
    });

    test('queues a casual match and reads its authoritative replay', () async {
      final requests = <http.Request>[];
      final client = GochyaApiClient(
        baseUri: Uri.parse('https://api.example.test'),
        httpClient: MockClient((request) async {
          requests.add(request);
          if (request.url.path == '/v1/matchmaking/queue') {
            return _jsonResponse({
              'matchId': _matchId,
              'status': 'completed',
            }, 200);
          }
          return _jsonResponse(_matchJson, 200);
        }),
      );

      final ticket = await client.queueCasualMatch(
        accessToken: 'access',
        idempotencyKey: '11111111-1111-4111-8111-111111111111',
      );
      final replay = await client.getMatch('access', ticket.matchId);

      expect(jsonDecode(requests.first.body), {'mode': 'casual'});
      expect(requests.last.method, 'GET');
      expect(requests.last.url.path, '/v1/match/$_matchId');
      expect(replay.outcomeFor('player-1'), MatchOutcome.win);
      expect(replay.outcomeFor('player-2'), MatchOutcome.loss);
      expect(replay.opponentOf('player-1'), 'player-2');
      expect(replay.rounds, hasLength(2));
      // Both players read the same replay and each sees the other's species.
      expect(replay.ownElement('player-1'), CreatureElement.earth);
      expect(replay.opponentElement('player-1'), CreatureElement.fire);
      expect(replay.ownElement('player-2'), CreatureElement.fire);
      expect(replay.opponentElement('player-2'), CreatureElement.earth);
    });

    test('confirms a match without a body and reads its reward', () async {
      late http.Request request;
      final client = GochyaApiClient(
        baseUri: Uri.parse('https://api.example.test'),
        httpClient: MockClient((value) async {
          request = value;
          return _jsonResponse({
            'matchId': _matchId,
            'outcome': 'win',
            'rewards': [
              {'currency': 'koins', 'amount': 30},
            ],
            'card': _techniqueJson,
            'confirmedAt': '2026-07-25T10:00:05Z',
          }, 200);
        }),
      );

      final confirmation = await client.confirmMatch(
        accessToken: 'access',
        matchId: _matchId,
      );

      expect(request.method, 'POST');
      expect(request.url.path, '/v1/match/$_matchId/confirm');
      expect(request.body, isEmpty);
      expect(confirmation.koins, 30);
      expect(confirmation.card?.rarity, TechniqueRarity.rare);
    });

    test('sends canonical breeding intent with its catalysts', () async {
      late http.Request request;
      final client = GochyaApiClient(
        baseUri: Uri.parse('https://api.example.test'),
        httpClient: MockClient((value) async {
          request = value;
          return _jsonResponse({
            'eggId': '44444444-4444-4444-8444-444444444444',
            'incubateUntil': '2026-07-25T18:00:00Z',
          }, 200);
        }),
      );

      final result = await client.breedPets(
        accessToken: 'access',
        parentAId: 'pet-a',
        parentBId: 'pet-b',
        catalysts: const [BreedingCatalyst.mutation],
        idempotencyKey: '11111111-1111-4111-8111-111111111111',
      );

      expect(request.url.path, '/v1/breeding/breed');
      expect(jsonDecode(request.body), {
        'parentA': 'pet-a',
        'parentB': 'pet-b',
        'catalysts': ['mutation'],
      });
      expect(result.incubateUntil, DateTime.parse('2026-07-25T18:00:00Z'));
    });

    test('refuses breeding a pet with itself', () {
      final client = GochyaApiClient(
        baseUri: Uri.parse('https://api.example.test'),
      );
      expect(
        () => client.breedPets(
          accessToken: 'access',
          parentAId: 'pet-a',
          parentBId: 'pet-a',
          catalysts: const [],
          idempotencyKey: '11111111-1111-4111-8111-111111111111',
        ),
        throwsArgumentError,
      );
    });

    test('reads the activity week and claims its daily card', () async {
      final requests = <http.Request>[];
      final client = GochyaApiClient(
        baseUri: Uri.parse('https://api.example.test'),
        httpClient: MockClient((request) async {
          requests.add(request);
          if (request.url.path == '/v1/me/activity/week') {
            return _jsonResponse([_dailyActivityJson], 200);
          }
          return _jsonResponse({
            'date': '2026-07-25',
            'card': _techniqueJson,
            'awarded': true,
          }, 200);
        }),
      );

      final week = await client.getActivityWeek('access');
      final reward = await client.claimActivityReward('access');

      expect(week.single.vitality, 118);
      expect(week.single.unlocksReward, isTrue);
      expect(week.single.snapshot.steps, 11240);
      expect(requests.last.method, 'POST');
      expect(requests.last.url.path, '/v1/me/activity/reward');
      expect(requests.last.body, isEmpty);
      expect(reward.awarded, isTrue);
    });

    test('rejects a day whose awarded vitality exceeds its total', () async {
      final client = GochyaApiClient(
        baseUri: Uri.parse('https://api.example.test'),
        httpClient: MockClient((request) async {
          return _jsonResponse([
            {..._dailyActivityJson, 'vitalityAwarded': 140},
          ], 200);
        }),
      );

      await expectLater(
        client.getActivityWeek('access'),
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
  'genome': {'element': 2},
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

const _matchId = '55555555-5555-4555-8555-555555555555';

const _cardIds = ['card-1', 'card-2', 'card-3', 'card-4', 'card-5'];

final _techniqueJson = <String, dynamic>{
  'id': 'card-1',
  'ownerId': 'player-1',
  'type': 1,
  'element': 2,
  'rarity': 2,
  'baseDamage': 24.5,
  'speed': 51.25,
  'staminaCost': 14,
  'critChance': 0.08,
  'effect': 0,
  'quality': 61,
  'createdAt': '2026-07-24T10:00:00Z',
};

final _matchJson = <String, dynamic>{
  'id': _matchId,
  'playerAId': 'player-1',
  'playerBId': 'player-2',
  'mode': 'casual',
  'elementA': 2,
  'elementB': 0,
  'loadoutRevisionA': 7,
  'loadoutRevisionB': 4,
  'result': {
    'winner': 'a',
    'rounds': [
      {
        'cardAIdx': 0,
        'cardBIdx': 2,
        'damageAToB': 18,
        'damageBToA': 11,
        'effectKind': 0,
        'effectValue': 0,
      },
      {
        'cardAIdx': 3,
        'cardBIdx': 1,
        'damageAToB': 24,
        'damageBToA': 9,
        'effectKind': 1,
        'effectValue': 1,
      },
    ],
    'finalHpA': 74,
    'finalHpB': 0,
    'seed': 42,
  },
  'createdAt': '2026-07-25T10:00:00Z',
};

final _dailyActivityJson = <String, dynamic>{
  'date': '2026-07-25',
  'snapshot': {
    'schemaVersion': 1,
    'timestampMillis': 1785060000000,
    'steps': 11240,
    'sleepMinutes': 431,
    'sleepQuality': 72,
    'activeCalories': 380,
    'workouts': [
      {'kind': 1, 'durationMinutes': 42, 'calories': 310},
    ],
    'averageHeartRate': 74,
    'highHeartZoneMinutes': 18,
    'meditationMinutes': 0,
    'stressLevel': 24,
    'floors': 6,
    'standHours': 11,
  },
  'vitality': 118,
  'vitalityAwarded': 118,
  'statGains': {'str': 1, 'agi': 2, 'end': 1, 'foc': 1},
  'goals': {'steps': 9000, 'sleepHours': 7.5, 'activeCalories': 420},
  'sourceMetadata': 'health_connect://phone',
  'updatedAt': '2026-07-25T20:00:00Z',
};

http.Response _jsonResponse(Object? body, int statusCode) {
  return http.Response.bytes(
    utf8.encode(jsonEncode(body)),
    statusCode,
    headers: {'content-type': 'application/json; charset=utf-8'},
  );
}
