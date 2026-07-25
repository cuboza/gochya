import 'profile_models.dart';

enum ShopCategory {
  care('care'),
  breeding('breeding');

  const ShopCategory(this.apiValue);

  factory ShopCategory.fromApi(String value) {
    return values.firstWhere(
      (category) => category.apiValue == value,
      orElse: () => throw FormatException('unsupported shop category $value'),
    );
  }

  final String apiValue;
}

enum ShopItemId {
  apple('apple', ShopCategory.care),
  steak('steak', ShopCategory.care),
  energyDrink('energy_drink', ShopCategory.care),
  soap('soap', ShopCategory.care),
  shampoo('shampoo', ShopCategory.care),
  loveCrystal('love_crystal', ShopCategory.breeding);

  const ShopItemId(this.apiValue, this.category);

  factory ShopItemId.fromApi(String value) {
    return values.firstWhere(
      (item) => item.apiValue == value,
      orElse: () => throw FormatException('unsupported shop item $value'),
    );
  }

  final String apiValue;
  final ShopCategory category;
}

class ShopCatalogItem {
  const ShopCatalogItem({
    required this.id,
    required this.category,
    required this.unitPrice,
    required this.isStackable,
  });

  factory ShopCatalogItem.fromJson(JsonMap json) {
    final id = ShopItemId.fromApi(requiredString(json, 'id'));
    final category = ShopCategory.fromApi(requiredString(json, 'category'));
    if (id.category != category) {
      throw const FormatException('shop item category is inconsistent');
    }
    if (requiredString(json, 'currency') != 'koins') {
      throw const FormatException('unsupported shop currency');
    }
    return ShopCatalogItem(
      id: id,
      category: category,
      unitPrice: rangedInt(json, 'unitPrice', min: 1),
      isStackable: requiredBool(json, 'isStackable'),
    );
  }

  final ShopItemId id;
  final ShopCategory category;
  final int unitPrice;
  final bool isStackable;
}

class ShopCatalog {
  const ShopCatalog({required this.items});

  factory ShopCatalog.fromJson(JsonMap json) {
    final rawItems = requiredList(json, 'items');
    if (rawItems.isEmpty || rawItems.length > 100) {
      throw const FormatException('shop catalog size is invalid');
    }
    final items = rawItems
        .map((value) => ShopCatalogItem.fromJson(asMap(value, 'items[]')))
        .toList(growable: false);
    if (items.map((item) => item.id).toSet().length != items.length) {
      throw const FormatException('shop catalog contains duplicate items');
    }
    return ShopCatalog(items: List.unmodifiable(items));
  }

  final List<ShopCatalogItem> items;
}

class OwnedShopItem {
  const OwnedShopItem({required this.itemId, required this.quantity});

  factory OwnedShopItem.fromJson(JsonMap json) {
    return OwnedShopItem(
      itemId: ShopItemId.fromApi(requiredString(json, 'itemId')),
      quantity: rangedInt(json, 'quantity', min: 1),
    );
  }

  final ShopItemId itemId;
  final int quantity;
}

class ShopInventory {
  const ShopInventory({required this.koins, required this.items});

  factory ShopInventory.fromJson(JsonMap json) {
    final items = requiredList(json, 'items')
        .map((value) => OwnedShopItem.fromJson(asMap(value, 'items[]')))
        .toList(growable: false);
    if (items.map((item) => item.itemId).toSet().length != items.length) {
      throw const FormatException('inventory contains duplicate items');
    }
    return ShopInventory(
      koins: rangedInt(json, 'koins', min: 0),
      items: List.unmodifiable(items),
    );
  }

  final int koins;
  final List<OwnedShopItem> items;

  int quantityOf(ShopItemId itemId) {
    for (final item in items) {
      if (item.itemId == itemId) {
        return item.quantity;
      }
    }
    return 0;
  }

  ShopInventory applying(ShopPurchase purchase) {
    final updated =
        <OwnedShopItem>[
          for (final item in items)
            if (item.itemId != purchase.itemId) item,
          OwnedShopItem(
            itemId: purchase.itemId,
            quantity: purchase.itemQuantity,
          ),
        ]..sort(
          (left, right) =>
              left.itemId.apiValue.compareTo(right.itemId.apiValue),
        );
    return ShopInventory(
      koins: purchase.koinsRemaining,
      items: List.unmodifiable(updated),
    );
  }
}

class ShopPurchase {
  const ShopPurchase({
    required this.itemId,
    required this.purchasedQuantity,
    required this.itemQuantity,
    required this.unitPriceKoins,
    required this.koinsSpent,
    required this.koinsRemaining,
    required this.purchasedAt,
  });

  factory ShopPurchase.fromJson(JsonMap json) {
    final purchasedQuantity = rangedInt(
      json,
      'purchasedQuantity',
      min: 1,
      max: 100,
    );
    final itemQuantity = rangedInt(json, 'itemQuantity', min: 1);
    final unitPrice = rangedInt(json, 'unitPriceKoins', min: 1);
    final koinsSpent = rangedInt(json, 'koinsSpent', min: 1);
    if (itemQuantity < purchasedQuantity ||
        koinsSpent != unitPrice * purchasedQuantity) {
      throw const FormatException('purchase totals are inconsistent');
    }
    return ShopPurchase(
      itemId: ShopItemId.fromApi(requiredString(json, 'itemId')),
      purchasedQuantity: purchasedQuantity,
      itemQuantity: itemQuantity,
      unitPriceKoins: unitPrice,
      koinsSpent: koinsSpent,
      koinsRemaining: rangedInt(json, 'koinsRemaining', min: 0),
      purchasedAt: requiredDateTime(json, 'purchasedAt'),
    );
  }

  final ShopItemId itemId;
  final int purchasedQuantity;
  final int itemQuantity;
  final int unitPriceKoins;
  final int koinsSpent;
  final int koinsRemaining;
  final DateTime purchasedAt;
}

class ShopSnapshot {
  const ShopSnapshot({required this.catalog, required this.inventory});

  final ShopCatalog catalog;
  final ShopInventory inventory;

  ShopSnapshot applying(ShopPurchase purchase) {
    return ShopSnapshot(
      catalog: catalog,
      inventory: inventory.applying(purchase),
    );
  }
}
