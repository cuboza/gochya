import 'profile_models.dart';
import 'technique_models.dart';

/// `MAX_ROUNDS` from `core/src/combat.rs`.
const maxCombatRounds = 20;

enum MatchOutcome {
  win('win', 'Победа'),
  loss('loss', 'Поражение'),
  draw('draw', 'Ничья');

  const MatchOutcome(this.apiValue, this.label);

  factory MatchOutcome.fromApi(String value) {
    return values.firstWhere(
      (outcome) => outcome.apiValue == value,
      orElse: () => throw FormatException('unsupported match outcome $value'),
    );
  }

  final String apiValue;
  final String label;
}

/// Queue acknowledgement. Casual matches resolve inside the queue call, so the
/// only status the server can report today is `completed`.
class MatchTicket {
  const MatchTicket({required this.matchId, required this.status});

  factory MatchTicket.fromJson(JsonMap json) {
    final status = requiredString(json, 'status');
    if (status != 'completed') {
      throw FormatException('unsupported match status $status');
    }
    return MatchTicket(
      matchId: requiredString(json, 'matchId'),
      status: status,
    );
  }

  final String matchId;
  final String status;
}

class MatchRound {
  const MatchRound({
    required this.cardAIdx,
    required this.cardBIdx,
    required this.damageAToB,
    required this.damageBToA,
    required this.effect,
    required this.effectValue,
  });

  factory MatchRound.fromJson(JsonMap json) {
    return MatchRound(
      cardAIdx: rangedInt(json, 'cardAIdx', min: 0, max: 4),
      cardBIdx: rangedInt(json, 'cardBIdx', min: 0, max: 4),
      damageAToB: rangedInt(json, 'damageAToB', min: 0, max: 65535),
      damageBToA: rangedInt(json, 'damageBToA', min: 0, max: 65535),
      effect: TechniqueEffect.fromApi(
        rangedInt(json, 'effectKind', min: 0, max: 5),
      ),
      effectValue: optionalDouble(json, 'effectValue', min: 0, max: 1000),
    );
  }

  final int cardAIdx;
  final int cardBIdx;
  final int damageAToB;
  final int damageBToA;
  final TechniqueEffect effect;
  final double effectValue;
}

/// Full server-computed replay. The client renders it and never recomputes it.
class MatchReplay {
  const MatchReplay({
    required this.id,
    required this.playerAId,
    required this.playerBId,
    required this.mode,
    required this.loadoutRevisionA,
    required this.loadoutRevisionB,
    required this.winner,
    required this.rounds,
    required this.finalHpA,
    required this.finalHpB,
    required this.seed,
    required this.createdAt,
  });

  factory MatchReplay.fromJson(JsonMap json) {
    final playerAId = requiredString(json, 'playerAId');
    final playerBId = requiredString(json, 'playerBId');
    if (playerAId == playerBId) {
      throw const FormatException('a match needs two distinct players');
    }
    final result = requiredMap(json, 'result');
    final winner = requiredString(result, 'winner');
    if (winner != 'a' && winner != 'b' && winner != 'draw') {
      throw FormatException('unsupported match winner $winner');
    }
    final rawRounds = requiredList(result, 'rounds');
    if (rawRounds.isEmpty || rawRounds.length > maxCombatRounds) {
      throw const FormatException('match replay round count is invalid');
    }
    return MatchReplay(
      id: requiredString(json, 'id'),
      playerAId: playerAId,
      playerBId: playerBId,
      mode: requiredString(json, 'mode'),
      loadoutRevisionA: rangedInt(json, 'loadoutRevisionA', min: 0),
      loadoutRevisionB: rangedInt(json, 'loadoutRevisionB', min: 0),
      winner: winner,
      rounds: List.unmodifiable(
        rawRounds
            .map((value) => MatchRound.fromJson(asMap(value, 'rounds[]')))
            .toList(growable: false),
      ),
      finalHpA: rangedInt(result, 'finalHpA', min: 0, max: 65535),
      finalHpB: rangedInt(result, 'finalHpB', min: 0, max: 65535),
      seed: rangedInt(result, 'seed', min: 0),
      createdAt: requiredDateTime(json, 'createdAt'),
    );
  }

  final String id;
  final String playerAId;
  final String playerBId;
  final String mode;
  final int loadoutRevisionA;
  final int loadoutRevisionB;
  final String winner;
  final List<MatchRound> rounds;
  final int finalHpA;
  final int finalHpB;
  final int seed;
  final DateTime createdAt;

  bool isPlayerA(String playerId) => playerId == playerAId;

  String opponentOf(String playerId) =>
      isPlayerA(playerId) ? playerBId : playerAId;

  MatchOutcome outcomeFor(String playerId) {
    if (playerId != playerAId && playerId != playerBId) {
      throw ArgumentError.value(playerId, 'playerId', 'is not in this match');
    }
    if (winner == 'draw') {
      return MatchOutcome.draw;
    }
    final playerWon = (winner == 'a') == isPlayerA(playerId);
    return playerWon ? MatchOutcome.win : MatchOutcome.loss;
  }

  int ownHp(String playerId) => isPlayerA(playerId) ? finalHpA : finalHpB;

  int opponentHp(String playerId) => isPlayerA(playerId) ? finalHpB : finalHpA;
}

class MatchSummary {
  const MatchSummary({
    required this.id,
    required this.opponentId,
    required this.mode,
    required this.outcome,
    required this.createdAt,
  });

  factory MatchSummary.fromJson(JsonMap json) {
    return MatchSummary(
      id: requiredString(json, 'id'),
      opponentId: requiredString(json, 'opponentId'),
      mode: requiredString(json, 'mode'),
      outcome: MatchOutcome.fromApi(requiredString(json, 'outcome')),
      createdAt: requiredDateTime(json, 'createdAt'),
    );
  }

  final String id;
  final String opponentId;
  final String mode;
  final MatchOutcome outcome;
  final DateTime createdAt;
}

class MatchReward {
  const MatchReward({required this.currency, required this.amount});

  factory MatchReward.fromJson(JsonMap json) {
    return MatchReward(
      currency: requiredString(json, 'currency'),
      amount: rangedInt(json, 'amount', min: 0),
    );
  }

  final String currency;
  final int amount;
}

/// Idempotent confirmation of an already-resolved match.
class MatchConfirmation {
  const MatchConfirmation({
    required this.matchId,
    required this.outcome,
    required this.rewards,
    required this.confirmedAt,
    this.card,
  });

  factory MatchConfirmation.fromJson(JsonMap json) {
    final rawCard = json['card'];
    final rewards = requiredList(json, 'rewards')
        .map((value) => MatchReward.fromJson(asMap(value, 'rewards[]')))
        .toList(growable: false);
    if (rewards.map((reward) => reward.currency).toSet().length !=
        rewards.length) {
      throw const FormatException('match rewards contain a duplicate currency');
    }
    return MatchConfirmation(
      matchId: requiredString(json, 'matchId'),
      outcome: MatchOutcome.fromApi(requiredString(json, 'outcome')),
      rewards: List.unmodifiable(rewards),
      card: rawCard == null
          ? null
          : TechniqueCardSummary.fromJson(asMap(rawCard, 'card')),
      confirmedAt: requiredDateTime(json, 'confirmedAt'),
    );
  }

  final String matchId;
  final MatchOutcome outcome;
  final List<MatchReward> rewards;
  final TechniqueCardSummary? card;
  final DateTime confirmedAt;

  int get koins {
    for (final reward in rewards) {
      if (reward.currency == 'koins') {
        return reward.amount;
      }
    }
    return 0;
  }
}
