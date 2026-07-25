import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gochya_companion/app/theme.dart';
import 'package:gochya_companion/core/models/breeding_models.dart';
import 'package:gochya_companion/core/models/onboarding_models.dart';
import 'package:gochya_companion/core/models/profile_models.dart';
import 'package:gochya_companion/core/models/shop_models.dart';
import 'package:gochya_companion/core/network/gochya_api_client.dart';
import 'package:gochya_companion/features/breeding/breeding_repository.dart';
import 'package:gochya_companion/features/breeding/breeding_screen.dart';

void main() {
  testWidgets('sends one breeding intent for two distinct adult parents', (
    tester,
  ) async {
    final repository = _FakeBreedingRepository();
    await _pumpBreeding(tester, repository);

    // The baby pet must not be offered as a parent.
    expect(find.textContaining('Моти'), findsNothing);
    expect(find.textContaining('Подходящих родителей: 2'), findsOneWidget);

    await _tap(tester, find.widgetWithText(ChoiceChip, 'Аква · ур. 31').first);
    await _tap(tester, find.widgetWithText(ChoiceChip, 'Искра · ур. 34').last);
    await _tap(tester, find.text('Катализатор мутации'));
    await _tap(tester, find.textContaining('Скрестить за 500 Koins'));
    await tester.pumpAndSettle();

    expect(repository.parentAId, 'pet-a');
    expect(repository.parentBId, 'pet-b');
    expect(repository.catalysts, [BreedingCatalyst.mutation]);
    expect(find.textContaining('Яйцо создано'), findsOneWidget);
  });

  testWidgets('keeps breeding disabled without Koins and a Love Crystal', (
    tester,
  ) async {
    final repository = _FakeBreedingRepository(
      inventory: const ShopInventory(koins: 100, items: []),
    );
    await _pumpBreeding(tester, repository);

    expect(
      tester
          .widget<FilledButton>(
            find.widgetWithText(FilledButton, 'Скрестить за 500 Koins'),
          )
          .onPressed,
      isNull,
    );
  });

  testWidgets('reuses the idempotency key after an uncertain breed', (
    tester,
  ) async {
    final repository = _FakeBreedingRepository(
      breedError: const ApiException(
        code: 'network_error',
        message: 'connection lost after send',
      ),
    );
    await _pumpBreeding(tester, repository);

    await _tap(tester, find.widgetWithText(ChoiceChip, 'Аква · ур. 31').first);
    await _tap(tester, find.widgetWithText(ChoiceChip, 'Искра · ур. 34').last);
    await _tap(tester, find.textContaining('Скрестить за 500 Koins'));
    await tester.pumpAndSettle();

    expect(find.textContaining('не спишет ресурсы дважды'), findsOneWidget);

    await _tap(tester, find.text('Повторить скрещивание'));
    await tester.pumpAndSettle();

    expect(repository.idempotencyKeys, hasLength(2));
    expect(repository.idempotencyKeys.first, repository.idempotencyKeys.last);
  });
}

Future<void> _tap(WidgetTester tester, Finder finder) async {
  await tester.ensureVisible(finder);
  await tester.pumpAndSettle();
  await tester.tap(finder);
  await tester.pump();
}

Future<void> _pumpBreeding(
  WidgetTester tester,
  BreedingRepository repository,
) async {
  await tester.binding.setSurfaceSize(const Size(1000, 3000));
  addTearDown(() => tester.binding.setSurfaceSize(null));
  await tester.pumpWidget(
    ProviderScope(
      overrides: [breedingRepositoryProvider.overrideWithValue(repository)],
      child: MaterialApp(
        theme: buildGochyaTheme(),
        home: const BreedingScreen(accessToken: 'access-token'),
      ),
    ),
  );
  await tester.pumpAndSettle();
}

class _FakeBreedingRepository implements BreedingRepository {
  _FakeBreedingRepository({this.breedError, this.inventory = _inventory});

  final Object? breedError;
  final ShopInventory inventory;
  final idempotencyKeys = <String>[];
  String? parentAId;
  String? parentBId;
  List<BreedingCatalyst>? catalysts;

  @override
  Future<BreedingSnapshot> load(String accessToken) async {
    expect(accessToken, 'access-token');
    return BreedingSnapshot(
      pets: [
        _baby,
        _parent('pet-a', 'Аква', 31),
        _parent('pet-b', 'Искра', 34),
      ],
      eggs: const [],
      inventory: inventory,
    );
  }

  @override
  Future<BreedingResult> breed({
    required String accessToken,
    required String parentAId,
    required String parentBId,
    required List<BreedingCatalyst> catalysts,
    required String idempotencyKey,
  }) async {
    idempotencyKeys.add(idempotencyKey);
    this.parentAId = parentAId;
    this.parentBId = parentBId;
    this.catalysts = catalysts;
    if (breedError case final error?) {
      throw error;
    }
    return BreedingResult(
      eggId: 'egg-1',
      incubateUntil: DateTime.parse('2026-07-25T18:00:00Z'),
    );
  }

  @override
  Future<HatchedPet> hatch({
    required String accessToken,
    required String eggId,
  }) => throw UnsupportedError('this test never hatches');
}

const _inventory = ShopInventory(
  koins: 500,
  items: [OwnedShopItem(itemId: ShopItemId.loveCrystal, quantity: 1)],
);

PetSummary _parent(String id, String name, int level) {
  return PetSummary(
    id: id,
    ownerId: 'player-1',
    genome: const {'element': 'Water'},
    name: name,
    stage: 'adult',
    level: level,
    xp: 12400,
    needs: const PetNeeds(hunger: 88, energy: 90, hygiene: 84, mood: 91),
    stats: const PetStats(strength: 21, agility: 19, endurance: 23, focus: 18),
    generation: 0,
    isActive: false,
    createdAt: DateTime.parse('2026-05-02T10:00:00Z'),
    isWeak: false,
    careRevision: 40,
    needsUpdatedAt: DateTime.parse('2026-07-25T12:00:00Z'),
  );
}

final _baby = PetSummary(
  id: 'pet-1',
  ownerId: 'player-1',
  genome: const {'element': 'Earth'},
  name: 'Моти',
  stage: 'baby',
  level: 4,
  xp: 320,
  needs: const PetNeeds(hunger: 81, energy: 72, hygiene: 65, mood: 94),
  stats: const PetStats(strength: 2, agility: 3, endurance: 4, focus: 5),
  generation: 1,
  isActive: true,
  createdAt: DateTime.parse('2026-07-20T10:00:00Z'),
  isWeak: false,
  careRevision: 9,
  needsUpdatedAt: DateTime.parse('2026-07-25T12:00:00Z'),
);
