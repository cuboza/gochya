import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/models/battle_models.dart';
import '../../core/network/api_providers.dart';
import '../../core/network/gochya_api_client.dart';
import '../session/session_request_runner.dart';

final battleRepositoryProvider = Provider<BattleRepository>(
  (ref) => ApiBattleRepository(
    api: ref.watch(apiClientProvider),
    sessionRunner: ref.watch(sessionRequestRunnerProvider),
  ),
);

final matchHistoryProvider = FutureProvider.autoDispose
    .family<List<MatchSummary>, String>((ref, accessToken) {
      return ref.watch(battleRepositoryProvider).history(accessToken);
    });

abstract interface class BattleRepository {
  /// Queues a casual match and returns the replay the server computed for it.
  ///
  /// The same [idempotencyKey] must be reused after an uncertain outcome: the
  /// server then returns the original match instead of starting a second one.
  Future<MatchReplay> queueCasual({
    required String accessToken,
    required String idempotencyKey,
  });

  Future<MatchConfirmation> confirm({
    required String accessToken,
    required String matchId,
  });

  Future<List<MatchSummary>> history(String accessToken);
}

class ApiBattleRepository implements BattleRepository {
  const ApiBattleRepository({required this.api, required this.sessionRunner});

  final GochyaApiClient api;
  final AuthenticatedRequestRunner sessionRunner;

  @override
  Future<MatchReplay> queueCasual({
    required String accessToken,
    required String idempotencyKey,
  }) async {
    final ticket = await sessionRunner.run(
      accessToken: accessToken,
      request: (token) => api.queueCasualMatch(
        accessToken: token,
        idempotencyKey: idempotencyKey,
      ),
    );
    return sessionRunner.run(
      accessToken: accessToken,
      request: (token) => api.getMatch(token, ticket.matchId),
    );
  }

  @override
  Future<MatchConfirmation> confirm({
    required String accessToken,
    required String matchId,
  }) {
    return sessionRunner.run(
      accessToken: accessToken,
      request: (token) =>
          api.confirmMatch(accessToken: token, matchId: matchId),
    );
  }

  @override
  Future<List<MatchSummary>> history(String accessToken) {
    return sessionRunner.run(
      accessToken: accessToken,
      request: (token) => api.getMatchHistory(token, limit: 20),
    );
  }
}
