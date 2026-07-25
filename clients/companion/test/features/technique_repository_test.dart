import 'package:flutter_test/flutter_test.dart';
import 'package:gochya_companion/core/models/technique_models.dart';
import 'package:gochya_companion/core/network/gochya_api_client.dart';
import 'package:gochya_companion/features/session/session_request_runner.dart';
import 'package:gochya_companion/features/techniques/technique_repository.dart';

void main() {
  test('collects every technique page before reading the loadout', () async {
    final api = _PagedTechniqueApi(pages: 3);
    final repository = ApiTechniqueRepository(
      api: api,
      sessionRunner: const _DirectRunner(),
    );

    final snapshot = await repository.load('access-token');

    expect(api.cursors, [null, 'cursor-1', 'cursor-2']);
    expect(snapshot.cards.map((card) => card.id), [
      'card-0',
      'card-1',
      'card-2',
    ]);
    expect(snapshot.loadout?.revision, 3);
    expect(snapshot.isBattleReady, isTrue);
  });

  test('treats a missing loadout as an empty, non-fatal state', () async {
    final api = _PagedTechniqueApi(
      pages: 1,
      loadoutError: const ApiException(
        statusCode: 404,
        code: 'loadout_not_found',
        message: 'no loadout yet',
      ),
    );
    final repository = ApiTechniqueRepository(
      api: api,
      sessionRunner: const _DirectRunner(),
    );

    final snapshot = await repository.load('access-token');

    expect(snapshot.loadout, isNull);
    expect(snapshot.isBattleReady, isFalse);
    expect(snapshot.cards, hasLength(1));
  });

  test('propagates a loadout failure that is not an empty state', () async {
    final api = _PagedTechniqueApi(
      pages: 1,
      loadoutError: const ApiException(
        statusCode: 503,
        code: 'core_unavailable',
        message: 'core is down',
      ),
    );
    final repository = ApiTechniqueRepository(
      api: api,
      sessionRunner: const _DirectRunner(),
    );

    await expectLater(
      repository.load('access-token'),
      throwsA(
        isA<ApiException>().having(
          (error) => error.code,
          'code',
          'core_unavailable',
        ),
      ),
    );
  });
}

class _DirectRunner implements AuthenticatedRequestRunner {
  const _DirectRunner();

  @override
  Future<T> run<T>({
    required String accessToken,
    required Future<T> Function(String accessToken) request,
  }) => request(accessToken);
}

class _PagedTechniqueApi extends GochyaApiClient {
  _PagedTechniqueApi({required this.pages, this.loadoutError})
    : super(baseUri: Uri.parse('https://api.example.test'));

  final int pages;
  final Object? loadoutError;
  final cursors = <String?>[];

  @override
  Future<TechniquePage> getTechniques(
    String accessToken, {
    int? limit,
    String? cursor,
  }) async {
    cursors.add(cursor);
    final index = cursors.length - 1;
    final isLast = cursors.length >= pages;
    return TechniquePage(
      items: [_card('card-$index')],
      nextCursor: isLast ? null : 'cursor-${index + 1}',
    );
  }

  @override
  Future<PetLoadout> getLoadout(String accessToken) async {
    if (loadoutError case final error?) {
      throw error;
    }
    return PetLoadout(
      petId: 'pet-1',
      cardIds: const ['card-0', 'card-1', 'card-2', 'card-3', 'card-4'],
      signatureIdx: 0,
      revision: 3,
      updatedAt: DateTime.parse('2026-07-25T10:00:00Z'),
    );
  }
}

TechniqueCardSummary _card(String id) {
  return TechniqueCardSummary(
    id: id,
    ownerId: 'player-1',
    type: TechniqueType.jab,
    element: CreatureElement.earth,
    rarity: TechniqueRarity.common,
    baseDamage: 14,
    speed: 50,
    staminaCost: 12,
    critChance: 0.05,
    effect: TechniqueEffect.none,
    effectValue: 0,
    quality: 40,
    createdAt: DateTime.parse('2026-07-24T10:00:00Z'),
  );
}
