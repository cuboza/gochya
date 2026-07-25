import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/identifiers/uuid_v4.dart';
import '../../core/models/onboarding_models.dart';
import '../../core/network/api_providers.dart';
import '../../core/network/gochya_api_client.dart';
import '../session/session_request_runner.dart';

final onboardingRepositoryProvider = Provider<OnboardingRepository>(
  (ref) => ApiOnboardingRepository(
    api: ref.watch(apiClientProvider),
    sessionRunner: ref.watch(sessionRequestRunnerProvider),
  ),
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
  const ApiOnboardingRepository({
    required this.api,
    required this.sessionRunner,
  });

  final GochyaApiClient api;
  final AuthenticatedRequestRunner sessionRunner;

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
    return sessionRunner.run(
      accessToken: accessToken,
      request: (token) => api.recordAgeGate(
        accessToken: token,
        birthDate: _formatDate(canonicalDate),
        idempotencyKey: idempotencyKey,
      ),
    );
  }

  @override
  Future<StarterEggResult> selectStarterEgg({
    required String accessToken,
    required StarterElement element,
    required String idempotencyKey,
  }) {
    return sessionRunner.run(
      accessToken: accessToken,
      request: (token) => api.selectStarterEgg(
        accessToken: token,
        element: element,
        idempotencyKey: idempotencyKey,
      ),
    );
  }

  @override
  Future<List<EggSummary>> loadEggs(String accessToken) {
    return sessionRunner.run(accessToken: accessToken, request: api.getEggs);
  }

  @override
  Future<HatchedPet> hatchEgg(String accessToken, String eggId) {
    return sessionRunner.run(
      accessToken: accessToken,
      request: (token) => api.hatchEgg(token, eggId),
    );
  }
}

String newIdempotencyKey() => newUuidV4();

String _formatDate(DateTime value) {
  String twoDigits(int number) => number.toString().padLeft(2, '0');
  return '${value.year.toString().padLeft(4, '0')}-'
      '${twoDigits(value.month)}-${twoDigits(value.day)}';
}
