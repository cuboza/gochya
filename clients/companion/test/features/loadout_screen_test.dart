import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gochya_companion/app/theme.dart';
import 'package:gochya_companion/core/models/technique_models.dart';
import 'package:gochya_companion/core/network/gochya_api_client.dart';
import 'package:gochya_companion/features/techniques/loadout_screen.dart';
import 'package:gochya_companion/features/techniques/technique_repository.dart';

void main() {
  testWidgets('equips exactly five selected cards with one signature', (
    tester,
  ) async {
    final repository = _FakeTechniqueRepository();
    await _pumpLoadout(tester, repository);

    expect(find.text('Лоадаут ещё не собран'), findsOneWidget);
    expect(
      tester
          .widget<FilledButton>(
            find.widgetWithText(FilledButton, 'Выбрано 0 из 5'),
          )
          .onPressed,
      isNull,
    );

    for (var index = 0; index < 5; index++) {
      await _tap(tester, find.text(_cards[index].type.label));
    }

    await _tap(tester, find.widgetWithText(ChoiceChip, 'Апперкот'));
    await _tap(tester, find.text('Экипировать пять карт'));
    await tester.pumpAndSettle();

    expect(repository.equippedCardIds, [
      'card-1',
      'card-2',
      'card-3',
      'card-4',
      'card-5',
    ]);
    expect(repository.equippedSignatureIdx, 2);
    expect(find.text('Лоадаут сохранён'), findsOneWidget);
  });

  testWidgets('never selects a sixth card', (tester) async {
    final repository = _FakeTechniqueRepository();
    await _pumpLoadout(tester, repository);

    for (final card in _cards) {
      await _tap(tester, find.text(card.type.label));
    }

    expect(find.text('Экипировать пять карт'), findsOneWidget);

    await _tap(tester, find.text('Экипировать пять карт'));
    await tester.pumpAndSettle();

    expect(repository.equippedCardIds, hasLength(5));
    expect(repository.equippedCardIds, isNot(contains('card-6')));
  });

  testWidgets('reuses the idempotency key after an uncertain equip', (
    tester,
  ) async {
    final repository = _FakeTechniqueRepository(
      equipError: const ApiException(
        code: 'network_error',
        message: 'connection lost after send',
      ),
    );
    await _pumpLoadout(tester, repository);

    for (var index = 0; index < 5; index++) {
      await _tap(tester, find.text(_cards[index].type.label));
    }

    await _tap(tester, find.text('Экипировать пять карт'));
    await tester.pumpAndSettle();

    expect(
      find.textContaining('Повтори — запрос идемпотентный'),
      findsOneWidget,
    );

    await _tap(tester, find.text('Экипировать пять карт'));
    await tester.pumpAndSettle();

    expect(repository.idempotencyKeys, hasLength(2));
    expect(repository.idempotencyKeys.first, repository.idempotencyKeys.last);
  });
}

/// The collection list is longer than the viewport, so every tap is scrolled
/// into view first.
Future<void> _tap(WidgetTester tester, Finder finder) async {
  await tester.ensureVisible(finder);
  await tester.pumpAndSettle();
  await tester.tap(finder);
  await tester.pump();
}

Future<void> _pumpLoadout(
  WidgetTester tester,
  TechniqueRepository repository,
) async {
  // A tall surface builds the whole lazy collection list, so selection taps do
  // not depend on scroll offsets.
  await tester.binding.setSurfaceSize(const Size(1000, 3000));
  addTearDown(() => tester.binding.setSurfaceSize(null));
  await tester.pumpWidget(
    ProviderScope(
      overrides: [techniqueRepositoryProvider.overrideWithValue(repository)],
      child: MaterialApp(
        theme: buildGochyaTheme(),
        home: const LoadoutScreen(accessToken: 'access-token'),
      ),
    ),
  );
  await tester.pumpAndSettle();
}

class _FakeTechniqueRepository implements TechniqueRepository {
  _FakeTechniqueRepository({this.equipError});

  final Object? equipError;
  final idempotencyKeys = <String>[];
  List<String>? equippedCardIds;
  int? equippedSignatureIdx;

  @override
  Future<LoadoutSnapshot> load(String accessToken) async {
    expect(accessToken, 'access-token');
    return LoadoutSnapshot(cards: _cards);
  }

  @override
  Future<PetLoadout> equip({
    required String accessToken,
    required List<String> cardIds,
    required int signatureIdx,
    required String idempotencyKey,
  }) async {
    idempotencyKeys.add(idempotencyKey);
    equippedCardIds = cardIds;
    equippedSignatureIdx = signatureIdx;
    if (equipError case final error?) {
      throw error;
    }
    return PetLoadout(
      petId: 'pet-1',
      cardIds: cardIds,
      signatureIdx: signatureIdx,
      revision: 1,
      updatedAt: DateTime.parse('2026-07-25T10:00:00Z'),
    );
  }
}

TechniqueCardSummary _card(String id, TechniqueType type) {
  return TechniqueCardSummary(
    id: id,
    ownerId: 'player-1',
    type: type,
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

final _cards = <TechniqueCardSummary>[
  _card('card-1', TechniqueType.jab),
  _card('card-2', TechniqueType.hook),
  _card('card-3', TechniqueType.uppercut),
  _card('card-4', TechniqueType.cross),
  _card('card-5', TechniqueType.kick),
  _card('card-6', TechniqueType.elbow),
];
