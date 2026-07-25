import 'package:flutter_test/flutter_test.dart';
import 'package:gochya_companion/core/models/battle_models.dart';
import 'package:gochya_companion/core/models/technique_models.dart';
import 'package:gochya_companion/core/network/gochya_api_client.dart';
import 'package:gochya_companion/features/battle/battle_repository.dart';
import 'package:gochya_companion/features/session/session_request_runner.dart';

void main() {
  test('a session rotation replays the queue with the same key', () async {
    final api = _RetryingBattleApi();
    final repository = ApiBattleRepository(
      api: api,
      sessionRunner: const _RetryOnceRunner(),
    );

    final replay = await repository.queueCasual(
      accessToken: 'expired-access',
      idempotencyKey: '11111111-1111-4111-8111-111111111111',
    );

    expect(api.queueTokens, ['expired-access', 'rotated-access']);
    expect(api.idempotencyKeys, [
      '11111111-1111-4111-8111-111111111111',
      '11111111-1111-4111-8111-111111111111',
    ]);
    expect(api.readMatchId, 'match-1');
    expect(replay.id, 'match-1');
  });
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

class _RetryingBattleApi extends GochyaApiClient {
  _RetryingBattleApi() : super(baseUri: Uri.parse('https://api.example.test'));

  final queueTokens = <String>[];
  final idempotencyKeys = <String>[];
  String? readMatchId;

  @override
  Future<MatchTicket> queueCasualMatch({
    required String accessToken,
    required String idempotencyKey,
  }) async {
    queueTokens.add(accessToken);
    idempotencyKeys.add(idempotencyKey);
    if (accessToken == 'expired-access') {
      throw const ApiException(
        statusCode: 401,
        code: 'token_expired',
        message: 'expired',
      );
    }
    return const MatchTicket(matchId: 'match-1', status: 'completed');
  }

  @override
  Future<MatchReplay> getMatch(String accessToken, String matchId) async {
    readMatchId = matchId;
    return MatchReplay(
      id: matchId,
      playerAId: 'player-1',
      playerBId: 'player-2',
      mode: 'casual',
      loadoutRevisionA: 7,
      loadoutRevisionB: 4,
      winner: 'a',
      rounds: const [
        MatchRound(
          cardAIdx: 0,
          cardBIdx: 2,
          damageAToB: 18,
          damageBToA: 11,
          effect: TechniqueEffect.none,
          effectValue: 0,
        ),
      ],
      finalHpA: 74,
      finalHpB: 0,
      seed: 42,
      createdAt: DateTime.parse('2026-07-25T10:00:00Z'),
    );
  }
}
