import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/models/shop_models.dart';
import '../../core/network/api_providers.dart';
import '../../core/network/gochya_api_client.dart';
import '../session/session_request_runner.dart';

final shopRepositoryProvider = Provider<ShopRepository>(
  (ref) => ApiShopRepository(
    api: ref.watch(apiClientProvider),
    sessionRunner: ref.watch(sessionRequestRunnerProvider),
  ),
);

abstract interface class ShopRepository {
  Future<ShopSnapshot> load(String accessToken);

  Future<ShopPurchase> purchase({
    required String accessToken,
    required ShopItemId itemId,
    required int quantity,
    required String idempotencyKey,
  });
}

class ApiShopRepository implements ShopRepository {
  const ApiShopRepository({required this.api, required this.sessionRunner});

  final GochyaApiClient api;
  final AuthenticatedRequestRunner sessionRunner;

  @override
  Future<ShopSnapshot> load(String accessToken) async {
    final values = await Future.wait<Object>([
      sessionRunner.run(accessToken: accessToken, request: api.getShopCatalog),
      sessionRunner.run(
        accessToken: accessToken,
        request: api.getShopInventory,
      ),
    ]);
    return ShopSnapshot(
      catalog: values[0] as ShopCatalog,
      inventory: values[1] as ShopInventory,
    );
  }

  @override
  Future<ShopPurchase> purchase({
    required String accessToken,
    required ShopItemId itemId,
    required int quantity,
    required String idempotencyKey,
  }) {
    return sessionRunner.run(
      accessToken: accessToken,
      request: (token) => api.purchaseShopItem(
        accessToken: token,
        itemId: itemId,
        quantity: quantity,
        idempotencyKey: idempotencyKey,
      ),
    );
  }
}
