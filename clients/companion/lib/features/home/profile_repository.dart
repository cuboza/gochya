import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/config/app_config.dart';
import '../../core/models/profile_models.dart';
import '../../core/network/gochya_api_client.dart';

final appConfigProvider = Provider<AppConfig>(
  (ref) => AppConfig.fromEnvironment(),
);

final apiClientProvider = Provider<GochyaApiClient>((ref) {
  final client = GochyaApiClient(
    baseUri: ref.watch(appConfigProvider).apiBaseUri,
  );
  ref.onDispose(client.close);
  return client;
});

final profileRepositoryProvider = Provider<ProfileRepository>(
  (ref) => ApiProfileRepository(ref.watch(apiClientProvider)),
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
  const ApiProfileRepository(this._api);

  final GochyaApiClient _api;

  @override
  Future<HomeSnapshot> loadHome(String accessToken) async {
    final profileFuture = _api.getProfile(accessToken);
    final petsFuture = _api.getPets(accessToken);
    final profile = await profileFuture;
    final pets = await petsFuture;
    return HomeSnapshot(profile: profile, pets: pets);
  }

  @override
  Future<LineageTree> loadLineage(String accessToken, String petId) {
    return _api.getLineage(accessToken, petId);
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
