import 'dart:math';

import 'package:flutter/material.dart';

import '../../app/theme.dart';
import '../../core/models/technique_models.dart';
import '../techniques/technique_content.dart';
import '../techniques/technique_glyph.dart';

/// The card being played this round: it flies in, flips face-up and glows in
/// its rarity colour, matching the "получить карту" beat in `ART_BIBLE.md`
/// §9.2. An opponent card stays face-down, because the match contract does not
/// expose which cards the other side owns.
class PlayedCard extends StatelessWidget {
  const PlayedCard({
    required this.card,
    required this.progress,
    required this.fromLeft,
    super.key,
  });

  /// `null` renders the face-down back used for the opponent.
  final TechniqueCardSummary? card;

  /// 0..1 across the round.
  final double progress;

  final bool fromLeft;

  static const _width = 62.0;
  static const _height = 86.0;

  @override
  Widget build(BuildContext context) {
    // In on an ease-out, hold through the impact, then drop away.
    final entry = Curves.easeOutBack.transform(
      (progress / 0.35).clamp(0.0, 1.0),
    );
    final exit = Curves.easeInCubic.transform(
      ((progress - 0.72) / 0.28).clamp(0.0, 1.0),
    );
    final flip = (progress / 0.45).clamp(0.0, 1.0);
    final showsFace = card != null && flip > 0.5;
    final opacity = (entry * (1 - exit)).clamp(0.0, 1.0);
    if (opacity <= 0.01) {
      return const SizedBox.shrink();
    }

    final frame = card?.rarity.frameColor ?? GochyaColors.muted;
    final slideFrom = fromLeft ? -46.0 : 46.0;
    return Opacity(
      opacity: opacity,
      child: Transform.translate(
        offset: Offset(slideFrom * (1 - entry), 18 * exit - 6 * entry),
        child: Transform(
          alignment: Alignment.center,
          transform: Matrix4.identity()
            ..setEntry(3, 2, 0.0015)
            ..rotateY((1 - flip) * pi)
            ..scaleByDouble(0.92 + 0.08 * entry, 0.92 + 0.08 * entry, 1, 1),
          child: _Face(card: showsFace ? card : null, frame: frame),
        ),
      ),
    );
  }
}

class _Face extends StatelessWidget {
  const _Face({required this.card, required this.frame});

  final TechniqueCardSummary? card;
  final Color frame;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: PlayedCard._width,
      height: PlayedCard._height,
      decoration: BoxDecoration(
        color: GochyaColors.background,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: frame, width: 1.6),
        boxShadow: [
          BoxShadow(color: frame.withValues(alpha: 0.45), blurRadius: 14),
        ],
      ),
      alignment: Alignment.center,
      child: card == null
          ? Icon(Icons.help_outline_rounded, color: frame, size: 26)
          : Column(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                TechniqueGlyph(type: card!.type, color: frame, size: 40),
                const SizedBox(height: 2),
                Text(
                  card!.type.label,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(
                    fontSize: 10,
                    fontWeight: FontWeight.w800,
                  ),
                ),
              ],
            ),
    );
  }
}
