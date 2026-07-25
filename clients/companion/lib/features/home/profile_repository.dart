import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/models/profile_models.dart';
import '../../core/network/api_providers.dart';
import '../../core/network/gochya_api_client.dart';
import '../session/session_request_runner.dart';

final profileRepositoryProvider = Provider<ProfileRepository>(
  (ref) => ApiProfileRepository(
    api: ref.watch(apiClientProvider),
    sessionRunner: ref.watch(sessionRequestRunnerProvider),
  ),
);

final homeSnapshotProvider = FutureProvider.autoDispose
    .family<HomeSnapshot, String>((ref, accessToken) {
      return ref.watch(profileRepositoryProvider).loadHome(accessToken);
    });

typedef LineageRequest = ({String accessToken, String petId});

final lineageProvider = FutureProvider.autoDispose
    .family<LineageTree, LineageRequest>((ref, request) {
      return ref
          .watch(profileRepositoryProvider)
          .loadLineage(request.accessToken, request.petId);
    });

abstract interface class ProfileRepository {
  Future<HomeSnapshot> loadHome(String accessToken);

  Future<LineageTree> loadLineage(String accessToken, String petId);
}

class ApiProfileRepository implements ProfileRepository {
  const ApiProfileRepository({required this.api, required this.sessionRunner});

  final GochyaApiClient api;
  final AuthenticatedRequestRunner sessionRunner;

  @override
  Future<HomeSnapshot> loadHome(String accessToken) async {
    final results = await Future.wait<Object>([
      sessionRunner.run(accessToken: accessToken, request: api.getProfile),
      sessionRunner.run(accessToken: accessToken, request: api.getPets),
    ]);
    return HomeSnapshot(
      profile: results[0] as PlayerProfile,
      pets: results[1] as List<PetSummary>,
    );
  }

  @override
  Future<LineageTree> loadLineage(String accessToken, String petId) {
    return sessionRunner.run(
      accessToken: accessToken,
      request: (token) => api.getLineage(token, petId),
    );
  }
}

class HomeSnapshot {
  const HomeSnapshot({required this.profile, required this.pets});

  final PlayerProfile profile;
  final List<PetSummary> pets;

  PetSummary? get activePet {
    final expectedId = profile.activePetId;
    if (expectedId != null) {
      for (final pet in pets) {
        if (pet.id == expectedId) {
          return pet;
        }
      }
    }
    for (final pet in pets) {
      if (pet.isActive) {
        return pet;
      }
    }
    return pets.firstOrNull;
  }
}
