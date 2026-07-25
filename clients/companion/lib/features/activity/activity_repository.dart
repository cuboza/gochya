import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/models/activity_models.dart';
import '../../core/network/api_providers.dart';
import '../../core/network/gochya_api_client.dart';
import '../session/session_request_runner.dart';

final activityRepositoryProvider = Provider<ActivityRepository>(
  (ref) => ApiActivityRepository(
    api: ref.watch(apiClientProvider),
    sessionRunner: ref.watch(sessionRequestRunnerProvider),
  ),
);

final activityWeekProvider = FutureProvider.autoDispose
    .family<List<DailyActivity>, String>((ref, accessToken) {
      return ref.watch(activityRepositoryProvider).week(accessToken);
    });

abstract interface class ActivityRepository {
  Future<List<DailyActivity>> week(String accessToken);

  /// Claims the deterministic daily card. The server decides whether it is
  /// granted, and a repeated claim returns the same card without a new grant.
  Future<ActivityRewardResult> claimReward(String accessToken);
}

class ApiActivityRepository implements ActivityRepository {
  const ApiActivityRepository({required this.api, required this.sessionRunner});

  final GochyaApiClient api;
  final AuthenticatedRequestRunner sessionRunner;

  @override
  Future<List<DailyActivity>> week(String accessToken) {
    return sessionRunner.run(
      accessToken: accessToken,
      request: api.getActivityWeek,
    );
  }

  @override
  Future<ActivityRewardResult> claimReward(String accessToken) {
    return sessionRunner.run(
      accessToken: accessToken,
      request: api.claimActivityReward,
    );
  }
}
