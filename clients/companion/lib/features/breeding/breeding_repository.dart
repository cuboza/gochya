import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/models/breeding_models.dart';
import '../../core/models/onboarding_models.dart';
import '../../core/models/profile_models.dart';
import '../../core/models/shop_models.dart';
import '../../core/network/api_providers.dart';
import '../../core/network/gochya_api_client.dart';
import '../session/session_request_runner.dart';

final breedingRepositoryProvider = Provider<BreedingRepository>(
  (ref) => ApiBreedingRepository(
    api: ref.watch(apiClientProvider),
    sessionRunner: ref.watch(sessionRequestRunnerProvider),
  ),
);

final breedingSnapshotProvider = FutureProvider.autoDispose
    .family<BreedingSnapshot, String>((ref, accessToken) {
      return ref.watch(breedingRepositoryProvider).load(accessToken);
    });

abstract interface class BreedingRepository {
  Future<BreedingSnapshot> load(String accessToken);

  Future<BreedingResult> breed({
    required String accessToken,
    required String parentAId,
    required String parentBId,
    required List<BreedingCatalyst> catalysts,
    required String idempotencyKey,
  });

  Future<HatchedPet> hatch({
    required String accessToken,
    required String eggId,
  });
}

class ApiBreedingRepository implements BreedingRepository {
  const ApiBreedingRepository({required this.api, required this.sessionRunner});

  final GochyaApiClient api;
  final AuthenticatedRequestRunner sessionRunner;

  @override
  Future<BreedingSnapshot> load(String accessToken) async {
    final values = await Future.wait<Object>([
      sessionRunner.run(accessToken: accessToken, request: api.getPets),
      sessionRunner.run(accessToken: accessToken, request: api.getEggs),
      sessionRunner.run(
        accessToken: accessToken,
        request: api.getShopInventory,
      ),
    ]);
    return BreedingSnapshot(
      pets: values[0] as List<PetSummary>,
      eggs: values[1] as List<EggSummary>,
      inventory: values[2] as ShopInventory,
    );
  }

  @override
  Future<BreedingResult> breed({
    required String accessToken,
    required String parentAId,
    required String parentBId,
    required List<BreedingCatalyst> catalysts,
    required String idempotencyKey,
  }) {
    return sessionRunner.run(
      accessToken: accessToken,
      request: (token) => api.breedPets(
        accessToken: token,
        parentAId: parentAId,
        parentBId: parentBId,
        catalysts: catalysts,
        idempotencyKey: idempotencyKey,
      ),
    );
  }

  @override
  Future<HatchedPet> hatch({
    required String accessToken,
    required String eggId,
  }) {
    return sessionRunner.run(
      accessToken: accessToken,
      request: (token) => api.hatchEgg(token, eggId),
    );
  }
}

class BreedingSnapshot {
  const BreedingSnapshot({
    required this.pets,
    required this.eggs,
    required this.inventory,
  });

  final List<PetSummary> pets;
  final List<EggSummary> eggs;
  final ShopInventory inventory;

  List<PetSummary> get eligibleParents =>
      pets.where(canBreed).toList(growable: false);

  int get loveCrystals => inventory.quantityOf(ShopItemId.loveCrystal);

  /// Mirrors the server's precondition set so the phone can explain a blocked
  /// request before spending a request on it.
  bool get canAffordBreeding =>
      inventory.koins >= breedCostKoins && loveCrystals >= 1;
}
