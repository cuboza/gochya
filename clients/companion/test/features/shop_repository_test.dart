import 'package:flutter_test/flutter_test.dart';
import 'package:gochya_companion/core/models/shop_models.dart';
import 'package:gochya_companion/core/network/gochya_api_client.dart';
import 'package:gochya_companion/features/session/session_request_runner.dart';
import 'package:gochya_companion/features/shop/shop_repository.dart';

void main() {
  test(
    'session retry preserves the original purchase idempotency key',
    () async {
      final api = _RetryingPurchaseApi();
      final repository = ApiShopRepository(
        api: api,
        sessionRunner: const _RetryOnceRunner(),
      );

      final purchase = await repository.purchase(
        accessToken: 'expired-access',
        itemId: ShopItemId.apple,
        quantity: 1,
        idempotencyKey: '11111111-1111-4111-8111-111111111111',
      );

      expect(api.accessTokens, ['expired-access', 'rotated-access']);
      expect(api.idempotencyKeys, [
        '11111111-1111-4111-8111-111111111111',
        '11111111-1111-4111-8111-111111111111',
      ]);
      expect(purchase.koinsRemaining, 80);
    },
  );
}

class _RetryOnceRunner implements AuthenticatedRequestRunner {
  const _RetryOnceRunner();

  @override
  Future<T> run<T>({
    required String accessToken,
    required Future<T> Function(String accessToken) request,
  }) async {
    try {
      return await request(accessToken);
    } on ApiException catch (error) {
      if (!error.isUnauthorized) {
        rethrow;
      }
      return request('rotated-access');
    }
  }
}

class _RetryingPurchaseApi extends GochyaApiClient {
  _RetryingPurchaseApi()
    : super(baseUri: Uri.parse('https://api.example.test'));

  final accessTokens = <String>[];
  final idempotencyKeys = <String>[];

  @override
  Future<ShopPurchase> purchaseShopItem({
    required String accessToken,
    required ShopItemId itemId,
    required int quantity,
    required String idempotencyKey,
  }) async {
    accessTokens.add(accessToken);
    idempotencyKeys.add(idempotencyKey);
    if (accessToken == 'expired-access') {
      throw const ApiException(
        statusCode: 401,
        code: 'token_expired',
        message: 'expired',
      );
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
