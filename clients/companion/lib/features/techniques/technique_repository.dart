import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/models/technique_models.dart';
import '../../core/network/api_providers.dart';
import '../../core/network/gochya_api_client.dart';
import '../session/session_request_runner.dart';

/// Bounded fan-out: the collection is paginated, so the phone reads at most
/// this many pages before it stops following cursors.
const _maxTechniquePages = 10;
const _techniquePageSize = 100;

final techniqueRepositoryProvider = Provider<TechniqueRepository>(
  (ref) => ApiTechniqueRepository(
    api: ref.watch(apiClientProvider),
    sessionRunner: ref.watch(sessionRequestRunnerProvider),
  ),
);

final loadoutSnapshotProvider = FutureProvider.autoDispose
    .family<LoadoutSnapshot, String>((ref, accessToken) {
      return ref.watch(techniqueRepositoryProvider).load(accessToken);
    });

abstract interface class TechniqueRepository {
  Future<LoadoutSnapshot> load(String accessToken);

  Future<PetLoadout> equip({
    required String accessToken,
    required List<String> cardIds,
    required int signatureIdx,
    required String idempotencyKey,
  });
}

class ApiTechniqueRepository implements TechniqueRepository {
  const ApiTechniqueRepository({
    required this.api,
    required this.sessionRunner,
  });

  final GochyaApiClient api;
  final AuthenticatedRequestRunner sessionRunner;

  @override
  Future<LoadoutSnapshot> load(String accessToken) async {
    final cards = await _readAllCards(accessToken);
    return LoadoutSnapshot(
      cards: List.unmodifiable(cards),
      loadout: await _readLoadout(accessToken),
    );
  }

  @override
  Future<PetLoadout> equip({
    required String accessToken,
    required List<String> cardIds,
    required int signatureIdx,
    required String idempotencyKey,
  }) {
    return sessionRunner.run(
      accessToken: accessToken,
      request: (token) => api.equipTechniques(
        accessToken: token,
        cardIds: cardIds,
        signatureIdx: signatureIdx,
        idempotencyKey: idempotencyKey,
      ),
    );
  }

  Future<List<TechniqueCardSummary>> _readAllCards(String accessToken) async {
    final cards = <TechniqueCardSummary>[];
    final seen = <String>{};
    String? cursor;
    for (var page = 0; page < _maxTechniquePages; page++) {
      final result = await sessionRunner.run(
        accessToken: accessToken,
        request: (token) =>
            api.getTechniques(token, limit: _techniquePageSize, cursor: cursor),
      );
      for (final card in result.items) {
        if (seen.add(card.id)) {
          cards.add(card);
        }
      }
      cursor = result.nextCursor;
      if (cursor == null) {
        break;
      }
    }
    return cards;
  }

  /// A player without an equipped loadout is a normal state, not a failure.
  Future<PetLoadout?> _readLoadout(String accessToken) async {
    try {
      return await sessionRunner.run(
        accessToken: accessToken,
        request: api.getLoadout,
      );
    } on ApiException catch (error) {
      if (error.code == 'loadout_not_found' ||
          error.code == 'active_pet_required') {
        return null;
      }
      rethrow;
    }
  }
}

class LoadoutSnapshot {
  const LoadoutSnapshot({required this.cards, this.loadout});

  final List<TechniqueCardSummary> cards;
  final PetLoadout? loadout;

  bool get isBattleReady => loadout != null;

  bool get canEquip => cards.length >= PetLoadout.loadoutSize;

  TechniqueCardSummary? cardById(String id) {
    for (final card in cards) {
      if (card.id == id) {
        return card;
      }
    }
    return null;
  }

  /// Equipped cards in loadout order; entries the collection no longer
  /// exposes are dropped rather than guessed.
  List<TechniqueCardSummary> get equippedCards {
    final current = loadout;
    if (current == null) {
      return const [];
    }
    return [for (final id in current.cardIds) ?cardById(id)];
  }
}
