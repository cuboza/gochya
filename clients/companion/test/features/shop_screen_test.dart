import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gochya_companion/app/theme.dart';
import 'package:gochya_companion/core/models/shop_models.dart';
import 'package:gochya_companion/core/network/gochya_api_client.dart';
import 'package:gochya_companion/features/shop/shop_repository.dart';
import 'package:gochya_companion/features/shop/shop_screen.dart';

void main() {
  testWidgets('applies only the canonical purchase result to the shop', (
    tester,
  ) async {
    final repository = _FakeShopRepository();
    await _pumpShop(tester, repository);

    expect(find.text('100 Koins'), findsOneWidget);
    expect(find.text('Яблоко'), findsOneWidget);
    expect(find.textContaining('В наличии: 2'), findsOneWidget);

    await tester.tap(find.widgetWithText(FilledButton, '20 K'));
    await tester.pumpAndSettle();

    expect(repository.purchasedItem, ShopItemId.apple);
    expect(repository.purchasedQuantity, 1);
    expect(
      repository.idempotencyKey,
      matches(
        RegExp(
          r'^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-'
          r'[89ab][0-9a-f]{3}-[0-9a-f]{12}$',
        ),
      ),
    );
    expect(find.text('80 Koins'), findsOneWidget);
    expect(find.textContaining('В наличии: 3'), findsOneWidget);
    expect(find.text('Яблоко: покупка подтверждена'), findsOneWidget);
  });

  testWidgets(
    'blocks another purchase until an uncertain result is refreshed',
    (tester) async {
      final repository = _FakeShopRepository(
        purchaseError: const ApiException(
          code: 'network_error',
          message: 'connection lost after send',
        ),
      );
      await _pumpShop(tester, repository);

      await tester.tap(find.widgetWithText(FilledButton, '20 K'));
      await tester.pumpAndSettle();

      expect(repository.purchaseCalls, 1);
      expect(
        find.textContaining(
          'Новая покупка заблокирована, пока сервер не подтвердит',
        ),
        findsOneWidget,
      );
      expect(
        tester
            .widget<FilledButton>(find.widgetWithText(FilledButton, '20 K'))
            .onPressed,
        isNull,
      );

      await tester.tap(find.text('Обновить магазин'));
      await tester.pumpAndSettle();

      expect(repository.loadCalls, 2);
      expect(
        find.textContaining(
          'Новая покупка заблокирована, пока сервер не подтвердит',
        ),
        findsNothing,
      );
      expect(
        tester
            .widget<FilledButton>(find.widgetWithText(FilledButton, '20 K'))
            .onPressed,
        isNotNull,
      );
    },
  );
}

Future<void> _pumpShop(WidgetTester tester, ShopRepository repository) async {
  await tester.pumpWidget(
    ProviderScope(
      overrides: [shopRepositoryProvider.overrideWithValue(repository)],
      child: MaterialApp(
        theme: buildGochyaTheme(),
        home: const ShopScreen(accessToken: 'access-token'),
      ),
    ),
  );
  await tester.pumpAndSettle();
}

class _FakeShopRepository implements ShopRepository {
  _FakeShopRepository({this.purchaseError});

  final Object? purchaseError;
  var loadCalls = 0;
  var purchaseCalls = 0;
  ShopItemId? purchasedItem;
  int? purchasedQuantity;
  String? idempotencyKey;

  @override
  Future<ShopSnapshot> load(String accessToken) async {
    loadCalls++;
    expect(accessToken, 'access-token');
    return _snapshot;
  }

  @override
  Future<ShopPurchase> purchase({
    required String accessToken,
    required ShopItemId itemId,
    required int quantity,
    required String idempotencyKey,
  }) async {
    purchaseCalls++;
    expect(accessToken, 'access-token');
    purchasedItem = itemId;
    purchasedQuantity = quantity;
    this.idempotencyKey = idempotencyKey;
    if (purchaseError case final error?) {
      throw error;
    }
    return ShopPurchase(
      itemId: itemId,
      purchasedQuantity: quantity,
      itemQuantity: 3,
      unitPriceKoins: 20,
      koinsSpent: 20,
      koinsRemaining: 80,
      purchasedAt: DateTime.parse('2026-07-25T12:00:00Z'),
    );
  }
}

const _snapshot = ShopSnapshot(
  catalog: ShopCatalog(
    items: [
      ShopCatalogItem(
        id: ShopItemId.apple,
        category: ShopCategory.care,
        unitPrice: 20,
        isStackable: true,
      ),
      ShopCatalogItem(
        id: ShopItemId.loveCrystal,
        category: ShopCategory.breeding,
        unitPrice: 200,
        isStackable: true,
      ),
    ],
  ),
  inventory: ShopInventory(
    koins: 100,
    items: [OwnedShopItem(itemId: ShopItemId.apple, quantity: 2)],
  ),
);
