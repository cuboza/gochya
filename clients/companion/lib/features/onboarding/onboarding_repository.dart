import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/identifiers/uuid_v4.dart';
import '../../core/models/onboarding_models.dart';
import '../../core/network/gochya_api_client.dart';
import '../home/profile_repository.dart';

final onboardingRepositoryProvider = Provider<OnboardingRepository>(
  (ref) => ApiOnboardingRepository(ref.watch(apiClientProvider)),
);

final onboardingEggsProvider = FutureProvider.autoDispose
    .family<List<EggSummary>, String>((ref, accessToken) {
      return ref.watch(onboardingRepositoryProvider).loadEggs(accessToken);
    });

abstract interface class OnboardingRepository {
  Future<AgeGateResult> recordAgeGate({
    required String accessToken,
    required DateTime birthDate,
    required String idempotencyKey,
  });

  Future<StarterEggResult> selectStarterEgg({
    required String accessToken,
    required StarterElement element,
    required String idempotencyKey,
  });

  Future<List<EggSummary>> loadEggs(String accessToken);

  Future<HatchedPet> hatchEgg(String accessToken, String eggId);
}

class ApiOnboardingRepository implements OnboardingRepository {
  const ApiOnboardingRepository(this._api);

  final GochyaApiClient _api;

  @override
  Future<AgeGateResult> recordAgeGate({
    required String accessToken,
    required DateTime birthDate,
    required String idempotencyKey,
  }) {
    final canonicalDate = DateTime.utc(
      birthDate.year,
      birthDate.month,
      birthDate.day,
    );
    return _api.recordAgeGate(
      accessToken: accessToken,
      birthDate: _formatDate(canonicalDate),
      idempotencyKey: idempotencyKey,
    );
  }

  @override
  Future<StarterEggResult> selectStarterEgg({
    required String accessToken,
    required StarterElement element,
    required String idempotencyKey,
  }) {
    return _api.selectStarterEgg(
      accessToken: accessToken,
      element: element,
      idempotencyKey: idempotencyKey,
    );
  }

  @override
  Future<List<EggSummary>> loadEggs(String accessToken) {
    return _api.getEggs(accessToken);
  }

  @override
  Future<HatchedPet> hatchEgg(String accessToken, String eggId) {
    return _api.hatchEgg(accessToken, eggId);
  }
}

String newIdempotencyKey() => newUuidV4();

String _formatDate(DateTime value) {
  String twoDigits(int number) => number.toString().padLeft(2, '0');
  return '${value.year.toString().padLeft(4, '0')}-'
      '${twoDigits(value.month)}-${twoDigits(value.day)}';
}
