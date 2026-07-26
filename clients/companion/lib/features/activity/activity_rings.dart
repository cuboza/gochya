import 'dart:math' as math;

import 'package:flutter/material.dart';

import '../../app/theme.dart';

/// One ring's worth of progress: how far the owner got against the goal the
/// server set for them.
class RingProgress {
  const RingProgress({
    required this.value,
    required this.goal,
    required this.color,
  });

  final double value;
  final double goal;
  final Color color;

  /// Unclamped ratio, so a day that beat its goal can still be announced as
  /// such even though the arc stops at a full turn.
  double get ratio => goal <= 0 ? 0 : value / goal;

  double get sweep => ratio.clamp(0.0, 1.0);
}

/// Three concentric activity rings, per `ART_BIBLE.md` §6.3.
///
/// The rings show the *owner's* body, not the pet's needs — those are a
/// separate scale drawn as arcs around the creature. Progress is read from the
/// server's goals; nothing here invents a number, so with no health source
/// connected every ring is simply empty.
class ActivityRings extends StatelessWidget {
  const ActivityRings({
    required this.steps,
    required this.sleep,
    required this.calories,
    required this.centerLabel,
    required this.centerCaption,
    this.size = 132,
    super.key,
  });

  final RingProgress steps;
  final RingProgress sleep;
  final RingProgress calories;
  final String centerLabel;
  final String centerCaption;
  final double size;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: size,
      height: size,
      child: CustomPaint(
        painter: _RingsPainter(
          rings: [steps, sleep, calories],
          track: GochyaColors.muted.withValues(alpha: 0.22),
        ),
        child: Center(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(
                centerLabel,
                style: Theme.of(context).textTheme.titleLarge?.copyWith(
                  fontWeight: FontWeight.w800,
                  height: 1,
                ),
              ),
              const SizedBox(height: 2),
              Text(
                centerCaption,
                style: Theme.of(context).textTheme.labelSmall?.copyWith(
                  color: GochyaColors.muted,
                  height: 1,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _RingsPainter extends CustomPainter {
  const _RingsPainter({required this.rings, required this.track});

  final List<RingProgress> rings;
  final Color track;

  // Sized so the innermost ring still leaves a clear disc for the Vitality
  // figure: at 132pt the inner radius lands at 32, giving the label ~56pt of
  // room. Tighter than this and the caption collides with the arc.
  static const _stroke = 8.0;
  static const _gap = 7.0;

  @override
  void paint(Canvas canvas, Size size) {
    final center = Offset(size.width / 2, size.height / 2);
    for (var index = 0; index < rings.length; index += 1) {
      final radius = size.width / 2 - _stroke / 2 - index * (_stroke + _gap);
      if (radius <= 0) {
        continue;
      }
      final rect = Rect.fromCircle(center: center, radius: radius);
      final base = Paint()
        ..style = PaintingStyle.stroke
        ..strokeWidth = _stroke
        ..strokeCap = StrokeCap.round
        ..color = track;
      canvas.drawArc(rect, 0, math.pi * 2, false, base);

      final sweep = rings[index].sweep;
      if (sweep <= 0) {
        continue;
      }
      final progress = Paint()
        ..style = PaintingStyle.stroke
        ..strokeWidth = _stroke
        ..strokeCap = StrokeCap.round
        ..color = rings[index].color;
      // Twelve o'clock, clockwise — the same direction the needs arcs fill.
      canvas.drawArc(rect, -math.pi / 2, math.pi * 2 * sweep, false, progress);
    }
  }

  @override
  bool shouldRepaint(_RingsPainter oldDelegate) {
    if (oldDelegate.track != track) {
      return true;
    }
    for (var index = 0; index < rings.length; index += 1) {
      if (oldDelegate.rings[index].sweep != rings[index].sweep ||
          oldDelegate.rings[index].color != rings[index].color) {
        return true;
      }
    }
    return false;
  }
}
