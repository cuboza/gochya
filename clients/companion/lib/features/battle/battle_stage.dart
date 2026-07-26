import 'dart:math';

import 'package:flutter/material.dart';

import '../../app/theme.dart';
import '../../core/models/battle_models.dart';
import '../../core/models/technique_models.dart';
import '../creatures/creature_rig.dart';
import '../creatures/rigged_creature.dart';
import 'played_card.dart';

/// Round-by-round playback of a finished match.
///
/// Everything shown here is read from the server replay. The only arithmetic
/// is presentational: starting HP is reconstructed as `finalHp + damage taken`
/// so the bars have somewhere to start, and the last frame always snaps back to
/// the authoritative `finalHp`.
///
/// Timing follows `ART_BIBLE.md` §9.2: one combat round is 400 ms of strike,
/// sparks and HP movement, with the played card flying in over it.
class BattleStage extends StatefulWidget {
  const BattleStage({
    required this.replay,
    required this.playerId,
    this.ownCards = const [],
    this.ownLoadoutRevision,
    super.key,
  });

  final MatchReplay replay;
  final String? playerId;

  /// The player's currently equipped cards, in loadout order.
  final List<TechniqueCardSummary> ownCards;

  /// Revision of [ownCards]. Card indices in the replay only refer to the same
  /// cards while this matches the revision the match was fought on.
  final int? ownLoadoutRevision;

  @override
  State<BattleStage> createState() => _BattleStageState();
}

class _BattleStageState extends State<BattleStage>
    with SingleTickerProviderStateMixin {
  static const _roundDuration = Duration(milliseconds: 400);
  static const _roundGap = Duration(milliseconds: 140);

  late final AnimationController _round = AnimationController(
    vsync: this,
    duration: _roundDuration,
  )..addStatusListener(_onRoundStatus);

  var _roundIndex = 0;
  var _finished = false;

  bool get _isPlayerA =>
      widget.playerId == null || widget.replay.isPlayerA(widget.playerId!);

  List<MatchRound> get _rounds => widget.replay.rounds;

  Iterable<int> get _damageTaken =>
      _rounds.map((round) => _isPlayerA ? round.damageBToA : round.damageAToB);

  Iterable<int> get _damageDealt =>
      _rounds.map((round) => _isPlayerA ? round.damageAToB : round.damageBToA);

  int get _ownFinalHp =>
      _isPlayerA ? widget.replay.finalHpA : widget.replay.finalHpB;

  int get _rivalFinalHp =>
      _isPlayerA ? widget.replay.finalHpB : widget.replay.finalHpA;

  late final int _ownStartHp =
      _ownFinalHp + _damageTaken.fold(0, (sum, value) => sum + value);
  late final int _rivalStartHp =
      _rivalFinalHp + _damageDealt.fold(0, (sum, value) => sum + value);

  CreatureElement get _ownElement => widget.playerId == null
      ? widget.replay.elementA
      : widget.replay.ownElement(widget.playerId!);

  CreatureElement get _rivalElement => widget.playerId == null
      ? widget.replay.elementB
      : widget.replay.opponentElement(widget.playerId!);

  /// Card indices are only meaningful while the loadout has not moved on.
  bool get _cardsStillMatch {
    final revision = widget.ownLoadoutRevision;
    if (revision == null || widget.ownCards.isEmpty) {
      return false;
    }
    final fought = _isPlayerA
        ? widget.replay.loadoutRevisionA
        : widget.replay.loadoutRevisionB;
    return revision == fought;
  }

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) => _start());
  }

  @override
  void didUpdateWidget(covariant BattleStage oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.replay.id != widget.replay.id) {
      _start();
    }
  }

  @override
  void dispose() {
    _round.dispose();
    super.dispose();
  }

  void _start() {
    if (!mounted) {
      return;
    }
    if (MediaQuery.disableAnimationsOf(context)) {
      setState(() {
        _roundIndex = _rounds.length - 1;
        _finished = true;
      });
      _round.value = 1;
      return;
    }
    setState(() {
      _roundIndex = 0;
      _finished = false;
    });
    _round.forward(from: 0);
  }

  Future<void> _onRoundStatus(AnimationStatus status) async {
    if (status != AnimationStatus.completed || !mounted) {
      return;
    }
    if (_roundIndex + 1 >= _rounds.length) {
      setState(() => _finished = true);
      return;
    }
    // A short beat between rounds lets the follow-through land before the next
    // wind-up starts.
    await Future<void>.delayed(_roundGap);
    if (!mounted || _finished) {
      return;
    }
    setState(() => _roundIndex++);
    _round.forward(from: 0);
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: _round,
      builder: (context, _) {
        final progress = _round.value;
        final round = _rounds[_roundIndex];
        final dealt = _isPlayerA ? round.damageAToB : round.damageBToA;
        final taken = _isPlayerA ? round.damageBToA : round.damageAToB;

        // Contact lands mid-round; HP and sparks follow it.
        final impact = ((progress - 0.35) / 0.25).clamp(0.0, 1.0);
        final ownHp = _hpAt(_ownStartHp, _damageTaken, impact);
        final rivalHp = _hpAt(_rivalStartHp, _damageDealt, impact);

        final ownCard = _cardFor(_isPlayerA ? round.cardAIdx : round.cardBIdx);
        final outcome = widget.playerId == null
            ? null
            : widget.replay.outcomeFor(widget.playerId!);

        return Column(
          children: [
            SizedBox(
              height: 220,
              child: Stack(
                alignment: Alignment.center,
                children: [
                  Positioned(
                    left: 0,
                    bottom: 0,
                    child: RiggedCreature(
                      element: _ownElement,
                      width: 150,
                      action: _finished
                          ? _outcomeAction(outcome, own: true)
                          : _actionFor(dealt: dealt, taken: taken),
                      strike: ownCard?.type ?? TechniqueType.jab,
                      actionProgress: progress,
                    ),
                  ),
                  Positioned(
                    right: 0,
                    bottom: 0,
                    child: Semantics(
                      label: 'Соперник, ${_rivalElement.label}',
                      child: RiggedCreature(
                        element: _rivalElement,
                        width: 150,
                        facingLeft: true,
                        action: _finished
                            ? _outcomeAction(outcome, own: false)
                            : _actionFor(dealt: taken, taken: dealt),
                        // The replay does not expose the opponent's cards, so
                        // their swing uses a neutral strike shape.
                        strike: TechniqueType.cross,
                        actionProgress: progress,
                      ),
                    ),
                  ),
                  if (!_finished) ...[
                    Align(
                      alignment: const Alignment(-0.72, -0.62),
                      child: PlayedCard(
                        card: ownCard,
                        progress: progress,
                        fromLeft: true,
                      ),
                    ),
                    Align(
                      alignment: const Alignment(0.72, -0.62),
                      child: PlayedCard(
                        card: null,
                        progress: progress,
                        fromLeft: false,
                      ),
                    ),
                    if (dealt > 0)
                      _Sparks(
                        progress: impact,
                        alignment: const Alignment(0.52, 0.1),
                        color: GochyaColors.secondary,
                      ),
                    if (taken > 0)
                      _Sparks(
                        progress: impact,
                        alignment: const Alignment(-0.52, 0.1),
                        color: GochyaColors.warning,
                      ),
                    if (dealt > 0)
                      _DamageNumber(
                        value: dealt,
                        progress: impact,
                        alignment: const Alignment(0.62, -0.3),
                        color: GochyaColors.success,
                      ),
                    if (taken > 0)
                      _DamageNumber(
                        value: taken,
                        progress: impact,
                        alignment: const Alignment(-0.62, -0.3),
                        color: GochyaColors.warning,
                      ),
                    if (round.effect != TechniqueEffect.none)
                      Align(
                        alignment: const Alignment(0, -0.92),
                        child: _EffectBadge(
                          label: round.effect.label,
                          progress: impact,
                        ),
                      ),
                  ],
                ],
              ),
            ),
            const SizedBox(height: 12),
            Row(
              children: [
                Expanded(
                  child: _HpBar(
                    label: 'Ты',
                    hp: ownHp,
                    maxHp: _ownStartHp,
                    color: GochyaColors.success,
                  ),
                ),
                const SizedBox(width: 14),
                Expanded(
                  child: _HpBar(
                    label: 'Соперник',
                    hp: rivalHp,
                    maxHp: _rivalStartHp,
                    color: GochyaColors.warning,
                    alignEnd: true,
                  ),
                ),
              ],
            ),
            const SizedBox(height: 10),
            Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                Text(
                  'Раунд ${_roundIndex + 1} из ${_rounds.length}',
                  style: const TextStyle(color: GochyaColors.muted),
                ),
                if (_finished)
                  TextButton.icon(
                    onPressed: _start,
                    icon: const Icon(Icons.replay_rounded, size: 18),
                    label: const Text('Смотреть снова'),
                  ),
              ],
            ),
          ],
        );
      },
    );
  }

  CreatureAction _actionFor({required int dealt, required int taken}) {
    if (dealt > 0) {
      return CreatureAction.strike;
    }
    return taken > 0 ? CreatureAction.hit : CreatureAction.idle;
  }

  CreatureAction _outcomeAction(MatchOutcome? outcome, {required bool own}) {
    if (outcome == null || outcome == MatchOutcome.draw) {
      return CreatureAction.idle;
    }
    final won = own == (outcome == MatchOutcome.win);
    return won ? CreatureAction.victory : CreatureAction.defeat;
  }

  TechniqueCardSummary? _cardFor(int index) {
    if (!_cardsStillMatch || index >= widget.ownCards.length) {
      return null;
    }
    return widget.ownCards[index];
  }

  /// HP after every completed round, plus the fraction of the current one.
  int _hpAt(int startHp, Iterable<int> damage, double impact) {
    final applied = damage.take(_roundIndex).fold(0, (sum, v) => sum + v);
    final current = _roundIndex < damage.length
        ? damage.elementAt(_roundIndex) * impact
        : 0.0;
    final value = startHp - applied - current;
    return max(0, value.round());
  }
}

class _EffectBadge extends StatelessWidget {
  const _EffectBadge({required this.label, required this.progress});

  final String label;
  final double progress;

  @override
  Widget build(BuildContext context) {
    if (progress <= 0) {
      return const SizedBox.shrink();
    }
    final scale = Curves.easeOutBack.transform(
      (progress * 2.2).clamp(0.0, 1.0),
    );
    return Transform.scale(
      scale: scale,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 4),
        decoration: BoxDecoration(
          color: GochyaColors.secondary.withValues(alpha: 0.22),
          borderRadius: BorderRadius.circular(20),
        ),
        child: Text(
          label,
          style: const TextStyle(
            color: GochyaColors.secondary,
            fontWeight: FontWeight.w800,
            fontSize: 12,
          ),
        ),
      ),
    );
  }
}

/// Impact sparks: short radial streaks that fade out after contact.
class _Sparks extends StatelessWidget {
  const _Sparks({
    required this.progress,
    required this.alignment,
    required this.color,
  });

  final double progress;
  final Alignment alignment;
  final Color color;

  @override
  Widget build(BuildContext context) {
    if (progress <= 0 || progress >= 1) {
      return const SizedBox.shrink();
    }
    return Align(
      alignment: alignment,
      child: CustomPaint(
        size: const Size.square(72),
        painter: _SparkPainter(progress: progress, color: color),
      ),
    );
  }
}

class _SparkPainter extends CustomPainter {
  const _SparkPainter({required this.progress, required this.color});

  final double progress;
  final Color color;

  static const _count = 7;

  @override
  void paint(Canvas canvas, Size size) {
    final center = size.center(Offset.zero);
    final reach = Curves.easeOutCubic.transform(progress);
    final paint = Paint()
      ..color = color.withValues(alpha: (1 - progress).clamp(0.0, 1.0))
      ..strokeCap = StrokeCap.round
      ..strokeWidth = 2.4;
    for (var index = 0; index < _count; index++) {
      final angle = index * 2 * pi / _count + progress * 0.4;
      final inner = center + Offset.fromDirection(angle, 8 + reach * 14);
      final outer = center + Offset.fromDirection(angle, 14 + reach * 26);
      canvas.drawLine(inner, outer, paint);
    }
  }

  @override
  bool shouldRepaint(_SparkPainter oldDelegate) =>
      oldDelegate.progress != progress || oldDelegate.color != color;
}

class _DamageNumber extends StatelessWidget {
  const _DamageNumber({
    required this.value,
    required this.progress,
    required this.alignment,
    required this.color,
  });

  final int value;
  final double progress;
  final Alignment alignment;
  final Color color;

  @override
  Widget build(BuildContext context) {
    if (progress <= 0) {
      return const SizedBox.shrink();
    }
    return Align(
      alignment: alignment,
      child: Transform.translate(
        offset: Offset(0, -30 * progress),
        child: Transform.scale(
          scale: 0.8 + 0.4 * Curves.easeOutBack.transform(progress),
          child: Opacity(
            opacity: (1 - progress).clamp(0.0, 1.0),
            child: Text(
              '-$value',
              style: TextStyle(
                color: color,
                fontWeight: FontWeight.w900,
                fontSize: 24,
                shadows: const [Shadow(color: Colors.black54, blurRadius: 6)],
              ),
            ),
          ),
        ),
      ),
    );
  }
}

class _HpBar extends StatelessWidget {
  const _HpBar({
    required this.label,
    required this.hp,
    required this.maxHp,
    required this.color,
    this.alignEnd = false,
  });

  final String label;
  final int hp;
  final int maxHp;
  final Color color;
  final bool alignEnd;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: alignEnd
          ? CrossAxisAlignment.end
          : CrossAxisAlignment.start,
      children: [
        Text(
          '$label · $hp',
          style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 12),
        ),
        const SizedBox(height: 4),
        LinearProgressIndicator(
          value: maxHp == 0 ? 0 : hp / maxHp,
          minHeight: 8,
          borderRadius: BorderRadius.circular(8),
          color: color,
          backgroundColor: color.withValues(alpha: 0.16),
        ),
      ],
    );
  }
}
