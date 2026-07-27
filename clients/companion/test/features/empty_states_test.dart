import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gochya_companion/app/theme.dart';
import 'package:gochya_companion/core/models/profile_models.dart';
import 'package:gochya_companion/core/models/shop_models.dart';
import 'package:gochya_companion/features/home/lineage_screen.dart';
import 'package:gochya_companion/features/home/profile_repository.dart';
import 'package:gochya_companion/features/shop/shop_repository.dart';
import 'package:gochya_companion/features/shop/shop_screen.dart';

/// `UX_UI.md` §8 (audit V8) asks every screen for a real empty state. These
/// cover the two that had none: a first-generation pet has no ancestors, and
/// the catalog can come back with nothing in it.
void main() {
  testWidgets('lineage explains that the род starts here', (tester) async {
    await _pumpLineage(tester, ancestors: 0);

    expect(find.text('Род начинается с него'), findsOneWidget);
    expect(find.textContaining('питомец первого поколения'), findsOneWidget);
    // Points somewhere useful instead of dead-ending.
    expect(find.textContaining('Бридинг'), findsOneWidget);
  });

  testWidgets('lineage draws the tree once ancestors exist', (tester) async {
    await _pumpLineage(tester, ancestors: 2);

    expect(find.text('Род начинается с него'), findsNothing);
  });

  testWidgets('an empty catalog reads as empty, not broken', (tester) async {
    await _pumpShop(tester, items: const []);

    expect(find.text('Прилавок пуст'), findsOneWidget);
    // The request succeeded, so no error wording may appear.
    expect(find.textContaining('Не удалось'), findsNothing);
  });

  testWidgets('an empty category shows no dangling heading', (tester) async {
    await _pumpShop(tester, items: [_item(ShopCategory.care)]);

    // Labels are literal here on purpose: the extension that produces them
    // is private to the shop screen, and a test that reimported it would stop
    // proving the heading a player actually sees.
    expect(find.text('Уход'), findsOneWidget);
    expect(find.text('Бридинг'), findsNothing);
  });
}

Future<void> _pumpLineage(WidgetTester tester, {required int ancestors}) async {
  await tester.binding.setSurfaceSize(const Size(1000, 1800));
  addTearDown(() => tester.binding.setSurfaceSize(null));
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        profileRepositoryProvider.overrideWithValue(
          _FakeProfileRepository(ancestors: ancestors),
        ),
      ],
      child: MaterialApp(
        theme: buildGochyaTheme(),
        home: LineageScreen(accessToken: 'access-token', pet: _pet),
      ),
    ),
  );
  await tester.pumpAndSettle();
}

Future<void> _pumpShop(
  WidgetTester tester, {
  required List<ShopCatalogItem> items,
}) async {
  await tester.binding.setSurfaceSize(const Size(1000, 1800));
  addTearDown(() => tester.binding.setSurfaceSize(null));
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        shopRepositoryProvider.overrideWithValue(_FakeShopRepository(items)),
      ],
      child: MaterialApp(
        theme: buildGochyaTheme(),
        home: const ShopScreen(accessToken: 'access-token'),
      ),
    ),
  );
  await tester.pumpAndSettle();
}

class _FakeProfileRepository implements ProfileRepository {
  const _FakeProfileRepository({required this.ancestors});

  final int ancestors;

  @override
  Future<HomeSnapshot> loadHome(String accessToken) =>
      throw UnsupportedError('this test never loads home');

  @override
  Future<LineageTree> loadLineage(String accessToken, String petId) async {
    return LineageTree(
      rootId: 'pet-1',
      maxDepth: ancestors == 0 ? 0 : 1,
      truncated: false,
      nodes: [
        _node('pet-1', 0),
        for (var index = 0; index < ancestors; index += 1)
          _node('ancestor-$index', 1),
      ],
    );
  }
}

class _FakeShopRepository implements ShopRepository {
  const _FakeShopRepository(this.items);

  final List<ShopCatalogItem> items;

  @override
  Future<ShopSnapshot> load(String accessToken) async {
    return ShopSnapshot(
      catalog: ShopCatalog(items: items),
      inventory: const ShopInventory(koins: 500, items: []),
    );
  }

  @override
  Future<ShopPurchase> purchase({
    required String accessToken,
    required ShopItemId itemId,
    required int quantity,
    required String idempotencyKey,
  }) => throw UnsupportedError('this test never buys');
}

ShopCatalogItem _item(ShopCategory category) {
  return ShopCatalogItem(
    id: ShopItemId.apple,
    category: category,
    unitPrice: 10,
    isStackable: true,
  );
}

LineageNode _node(String id, int depth) {
  return LineageNode(
    id: id,
    genome: const {'element': 2},
    stage: 'adult',
    level: 30,
    generation: 1,
    mutatedGenes: 0,
    ancestorDepth: depth,
    name: 'Питомец $id',
  );
}

final _pet = PetSummary(
  id: 'pet-1',
  ownerId: 'player-1',
  genome: const {'element': 2},
  name: 'Моти',
  stage: 'baby',
  level: 4,
  xp: 320,
  needs: const PetNeeds(hunger: 80, energy: 70, hygiene: 60, mood: 50),
  stats: const PetStats(strength: 2, agility: 3, endurance: 4, focus: 5),
  generation: 1,
  isActive: true,
  createdAt: DateTime.parse('2026-07-20T10:00:00Z'),
  isWeak: false,
  careRevision: 9,
  needsUpdatedAt: DateTime.parse('2026-07-26T12:00:00Z'),
);
