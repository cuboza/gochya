import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../app/gochya_loader.dart';
import '../../app/theme.dart';
import '../../core/identifiers/uuid_v4.dart';
import '../../core/models/shop_models.dart';
import '../../core/network/gochya_api_client.dart';
import 'shop_repository.dart';

class ShopScreen extends ConsumerStatefulWidget {
  const ShopScreen({required this.accessToken, super.key});

  final String accessToken;

  @override
  ConsumerState<ShopScreen> createState() => _ShopScreenState();
}

class _ShopScreenState extends ConsumerState<ShopScreen> {
  ShopSnapshot? _snapshot;
  Object? _loadError;
  ShopItemId? _purchasingItem;
  String? _purchaseError;
  var _isLoading = true;
  var _purchaseOutcomeUncertain = false;

  @override
  void initState() {
    super.initState();
    Future<void>.microtask(_load);
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Магазин')),
      body: switch ((_isLoading, _snapshot, _loadError)) {
        (true, null, _) => const GochyaLoader(caption: 'Открываем магазин…'),
        (_, null, final error?) => _ShopLoadError(
          message: _messageFor(error),
          onRetry: _load,
        ),
        (_, null, null) => const GochyaLoader(caption: 'Открываем магазин…'),
        (_, final snapshot?, _) => RefreshIndicator(
          onRefresh: _load,
          child: ListView(
            padding: const EdgeInsets.fromLTRB(20, 12, 20, 120),
            children: [
              _WalletCard(koins: snapshot.inventory.koins),
              if (_purchaseOutcomeUncertain) ...[
                const SizedBox(height: 16),
                _UncertainPurchaseCard(onRefresh: _load),
              ],
              if (_purchaseError != null) ...[
                const SizedBox(height: 12),
                Text(
                  _purchaseError!,
                  textAlign: TextAlign.center,
                  style: TextStyle(color: Theme.of(context).colorScheme.error),
                ),
              ],
              if (snapshot.catalog.items.isEmpty) ...[
                const SizedBox(height: 24),
                const _EmptyCatalogCard(),
              ],
              // A category heading with nothing under it reads as a broken
              // screen, so an empty section is skipped rather than titled.
              for (final category in ShopCategory.values)
                if (snapshot.catalog.items.any(
                  (item) => item.category == category,
                )) ...[
                  const SizedBox(height: 24),
                  Text(
                    category.label,
                    style: Theme.of(context).textTheme.titleLarge?.copyWith(
                      fontWeight: FontWeight.w800,
                    ),
                  ),
                  const SizedBox(height: 12),
                  for (final item in snapshot.catalog.items.where(
                    (item) => item.category == category,
                  )) ...[
                    _ShopItemCard(
                      item: item,
                      ownedQuantity: snapshot.inventory.quantityOf(item.id),
                      koins: snapshot.inventory.koins,
                      isPurchasing: _purchasingItem == item.id,
                      purchaseDisabled:
                          _purchasingItem != null || _purchaseOutcomeUncertain,
                      onPurchase: () => _purchase(item),
                    ),
                    const SizedBox(height: 12),
                  ],
                ],
            ],
          ),
        ),
      },
    );
  }

  Future<void> _load() async {
    if (mounted) {
      setState(() {
        _isLoading = true;
        _loadError = null;
      });
    }
    try {
      final snapshot = await ref
          .read(shopRepositoryProvider)
          .load(widget.accessToken);
      if (!mounted) {
        return;
      }
      setState(() {
        _snapshot = snapshot;
        _isLoading = false;
        _purchaseOutcomeUncertain = false;
        _purchaseError = null;
      });
    } on Object catch (error) {
      if (!mounted) {
        return;
      }
      setState(() {
        _isLoading = false;
        _loadError = error;
      });
    }
  }

  Future<void> _purchase(ShopCatalogItem item) async {
    setState(() {
      _purchasingItem = item.id;
      _purchaseError = null;
    });
    try {
      final purchase = await ref
          .read(shopRepositoryProvider)
          .purchase(
            accessToken: widget.accessToken,
            itemId: item.id,
            quantity: 1,
            idempotencyKey: newUuidV4(),
          );
      if (!mounted) {
        return;
      }
      setState(() {
        _snapshot = _snapshot!.applying(purchase);
        _purchaseOutcomeUncertain = false;
      });
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('${item.id.label}: покупка подтверждена')),
      );
    } on ApiException catch (error) {
      if (!mounted) {
        return;
      }
      final isDeterministicClientResult =
          error.statusCode != null &&
          error.statusCode! >= 400 &&
          error.statusCode! < 500;
      setState(() {
        _purchaseOutcomeUncertain = !isDeterministicClientResult;
        _purchaseError = _purchaseMessage(error);
      });
    } on Object {
      if (!mounted) {
        return;
      }
      setState(() {
        _purchaseOutcomeUncertain = true;
        _purchaseError =
            'Исход покупки неизвестен. Обновите магазин перед новой покупкой.';
      });
    } finally {
      if (mounted) {
        setState(() {
          _purchasingItem = null;
        });
      }
    }
  }

  String _purchaseMessage(ApiException error) {
    return switch (error.code) {
      'insufficient_koins' => 'Недостаточно Koins для этой покупки.',
      'inventory_limit' => 'Достигнут лимит этого предмета.',
      'shop_item_invalid' ||
      'quantity_invalid' => 'Этот товар сейчас недоступен.',
      'request_timeout' || 'network_error' =>
        'Исход покупки неизвестен. Обновите магазин перед новой покупкой.',
      _ when error.statusCode != null && error.statusCode! >= 500 =>
        'Сервер не подтвердил исход покупки. Обновите магазин.',
      _ => 'Не удалось выполнить покупку.',
    };
  }

  String _messageFor(Object error) {
    if (error is ApiException) {
      return switch (error.code) {
        'request_timeout' || 'network_error' => 'Магазин недоступен без сети.',
        _ => 'Не удалось загрузить авторитетный каталог.',
      };
    }
    return 'Не удалось загрузить авторитетный каталог.';
  }
}

class _WalletCard extends StatelessWidget {
  const _WalletCard({required this.koins});

  final int koins;

  @override
  Widget build(BuildContext context) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Row(
          children: [
            const CircleAvatar(
              backgroundColor: GochyaColors.secondary,
              foregroundColor: Colors.black,
              child: Icon(Icons.monetization_on_rounded),
            ),
            const SizedBox(width: 14),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    'Баланс',
                    style: Theme.of(
                      context,
                    ).textTheme.bodyMedium?.copyWith(color: GochyaColors.muted),
                  ),
                  Text(
                    '$koins Koins',
                    style: Theme.of(context).textTheme.headlineSmall?.copyWith(
                      fontWeight: FontWeight.w800,
                    ),
                  ),
                ],
              ),
            ),
            const Icon(Icons.verified_outlined, color: GochyaColors.success),
          ],
        ),
      ),
    );
  }
}

class _ShopItemCard extends StatelessWidget {
  const _ShopItemCard({
    required this.item,
    required this.ownedQuantity,
    required this.koins,
    required this.isPurchasing,
    required this.purchaseDisabled,
    required this.onPurchase,
  });

  final ShopCatalogItem item;
  final int ownedQuantity;
  final int koins;
  final bool isPurchasing;
  final bool purchaseDisabled;
  final VoidCallback onPurchase;

  @override
  Widget build(BuildContext context) {
    final canAfford = koins >= item.unitPrice;
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Row(
          children: [
            Container(
              width: 52,
              height: 52,
              decoration: BoxDecoration(
                color: item.id.color.withValues(alpha: 0.18),
                borderRadius: BorderRadius.circular(16),
              ),
              child: Icon(item.id.icon, color: item.id.color),
            ),
            const SizedBox(width: 14),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    item.id.label,
                    style: Theme.of(context).textTheme.titleMedium?.copyWith(
                      fontWeight: FontWeight.w800,
                    ),
                  ),
                  const SizedBox(height: 4),
                  Text(
                    '${item.id.description} · В наличии: $ownedQuantity',
                    style: Theme.of(
                      context,
                    ).textTheme.bodySmall?.copyWith(color: GochyaColors.muted),
                  ),
                ],
              ),
            ),
            const SizedBox(width: 12),
            FilledButton.tonal(
              onPressed: purchaseDisabled || !canAfford ? null : onPurchase,
              child: isPurchasing
                  ? const SizedBox.square(
                      dimension: 18,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    )
                  : Text(canAfford ? '${item.unitPrice} K' : 'Мало K'),
            ),
          ],
        ),
      ),
    );
  }
}

class _UncertainPurchaseCard extends StatelessWidget {
  const _UncertainPurchaseCard({required this.onRefresh});

  final Future<void> Function() onRefresh;

  @override
  Widget build(BuildContext context) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          children: [
            const Icon(Icons.sync_problem_rounded, color: GochyaColors.warning),
            const SizedBox(height: 8),
            const Text(
              'Новая покупка заблокирована, пока сервер не подтвердит '
              'баланс и инвентарь.',
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 12),
            FilledButton.icon(
              onPressed: onRefresh,
              icon: const Icon(Icons.refresh_rounded),
              label: const Text('Обновить магазин'),
            ),
          ],
        ),
      ),
    );
  }
}

/// Shown when the server returns a catalog with nothing in it. That is not an
/// error — the request succeeded — so it must not look like one.
class _EmptyCatalogCard extends StatelessWidget {
  const _EmptyCatalogCard();

  @override
  Widget build(BuildContext context) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          children: [
            const Icon(
              Icons.storefront_outlined,
              size: 40,
              color: GochyaColors.muted,
            ),
            const SizedBox(height: 14),
            Text(
              'Прилавок пуст',
              style: Theme.of(
                context,
              ).textTheme.titleLarge?.copyWith(fontWeight: FontWeight.w800),
            ),
            const SizedBox(height: 8),
            const Text(
              'Сервер пока не прислал ни одного товара. Потяните экран, чтобы '
              'проверить ещё раз.',
              textAlign: TextAlign.center,
            ),
          ],
        ),
      ),
    );
  }
}

class _ShopLoadError extends StatelessWidget {
  const _ShopLoadError({required this.message, required this.onRetry});

  final String message;
  final Future<void> Function() onRetry;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.storefront_outlined, size: 64),
            const SizedBox(height: 16),
            Text(message, textAlign: TextAlign.center),
            const SizedBox(height: 16),
            FilledButton.icon(
              onPressed: onRetry,
              icon: const Icon(Icons.refresh_rounded),
              label: const Text('Повторить'),
            ),
          ],
        ),
      ),
    );
  }
}

extension on ShopCategory {
  String get label => switch (this) {
    ShopCategory.care => 'Уход',
    ShopCategory.breeding => 'Бридинг',
  };
}

extension on ShopItemId {
  String get label => switch (this) {
    ShopItemId.apple => 'Яблоко',
    ShopItemId.steak => 'Стейк',
    ShopItemId.energyDrink => 'Энергетик',
    ShopItemId.soap => 'Мыло',
    ShopItemId.shampoo => 'Шампунь',
    ShopItemId.loveCrystal => 'Кристалл любви',
  };

  String get description => switch (this) {
    ShopItemId.apple => 'Лёгкая еда',
    ShopItemId.steak => 'Сытная еда',
    ShopItemId.energyDrink => 'Восстановление энергии',
    ShopItemId.soap => 'Базовая чистота',
    ShopItemId.shampoo => 'Глубокая чистота',
    ShopItemId.loveCrystal => 'Расходник для бридинга',
  };

  IconData get icon => switch (this) {
    ShopItemId.apple => Icons.apple_rounded,
    ShopItemId.steak => Icons.restaurant_rounded,
    ShopItemId.energyDrink => Icons.bolt_rounded,
    ShopItemId.soap => Icons.soap_rounded,
    ShopItemId.shampoo => Icons.shower_rounded,
    ShopItemId.loveCrystal => Icons.favorite_rounded,
  };

  Color get color => switch (this) {
    ShopItemId.apple => GochyaColors.hunger,
    ShopItemId.steak => GochyaColors.warning,
    ShopItemId.energyDrink => GochyaColors.energy,
    ShopItemId.soap || ShopItemId.shampoo => GochyaColors.hygiene,
    ShopItemId.loveCrystal => GochyaColors.mood,
  };
}
