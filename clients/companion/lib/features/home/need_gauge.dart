import 'dart:math' as math;

import 'package:flutter/material.dart';

import '../../app/theme.dart';
import 'need_indicator.dart' show lowNeedThreshold;

/// One need as a ring with its icon inside, per the layout in
/// `docs/06-art/artifacts/GOCHYA_UI_CONCEPT_V1.md` and the arc language of
/// `ART_BIBLE.md` §5 and §6.3.
///
/// Four rings in a row read at a glance and cost one line; four stacked bars
/// cost four and pushed the daily care actions off screen.
///
/// A need under [lowNeedThreshold] still pulses *and* still carries a static
/// marker — the ring turns into a warning dot that survives reduce motion,
/// because movement may never be the only channel.
class NeedGauge extends StatefulWidget {
  const NeedGauge({
    required this.label,
    required this.icon,
    required this.value,
    required this.color,
    super.key,
  });

  final String label;
  final IconData icon;
  final int value;
  final Color color;

  bool get isLow => value < lowNeedThreshold;

  @override
  State<NeedGauge> createState() => _NeedGaugeState();
}

class _NeedGaugeState extends State<NeedGauge>
    with SingleTickerProviderStateMixin {
  late final AnimationController _pulse = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 1200),
  );

  bool _running = false;

  @override
  void dispose() {
    _pulse.dispose();
    super.dispose();
  }

  void _syncPulse({required bool wanted}) {
    if (wanted == _running) {
      return;
    }
    _running = wanted;
    if (wanted) {
      _pulse.repeat(reverse: true);
    } else {
      _pulse.stop();
      _pulse.value = 0;
    }
  }

  @override
  Widget build(BuildContext context) {
    final reducedMotion = MediaQuery.disableAnimationsOf(context);
    _syncPulse(wanted: widget.isLow && !reducedMotion);
    final scaler = MediaQuery.textScalerOf(context);
    // The ring grows with the type so the icon inside never gets cramped.
    final diameter = scaler.scale(52).clamp(52.0, 92.0);

    return Semantics(
      label: widget.isLow
          ? '${widget.label}: ${widget.value} из 100, мало'
          : '${widget.label}: ${widget.value} из 100',
      child: ExcludeSemantics(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            AnimatedBuilder(
              animation: _pulse,
              builder: (context, child) {
                // Never fades far enough to become unreadable.
                final alpha = _running ? 1 - _pulse.value * 0.45 : 1.0;
                return Opacity(opacity: alpha, child: child);
              },
              child: SizedBox(
                width: diameter,
                height: diameter,
                child: CustomPaint(
                  painter: _GaugePainter(
                    progress: widget.value / 100,
                    color: widget.color,
                    track: GochyaColors.muted.withValues(alpha: 0.22),
                  ),
                  child: Center(
                    child: Icon(
                      widget.icon,
                      size: diameter * 0.38,
                      color: widget.color,
                    ),
                  ),
                ),
              ),
            ),
            const SizedBox(height: 6),
            Text(
              '${widget.value}%',
              style: Theme.of(context).textTheme.labelLarge?.copyWith(
                fontWeight: FontWeight.w800,
                color: widget.isLow ? widget.color : null,
              ),
            ),
            Text(
              widget.label,
              textAlign: TextAlign.center,
              style: Theme.of(
                context,
              ).textTheme.labelSmall?.copyWith(color: GochyaColors.muted),
            ),
            if (widget.isLow)
              Text(
                'мало',
                style: Theme.of(context).textTheme.labelSmall?.copyWith(
                  color: widget.color,
                  fontWeight: FontWeight.w700,
                ),
              ),
          ],
        ),
      ),
    );
  }
}

class _GaugePainter extends CustomPainter {
  const _GaugePainter({
    required this.progress,
    required this.color,
    required this.track,
  });

  final double progress;
  final Color color;
  final Color track;

  @override
  void paint(Canvas canvas, Size size) {
    const stroke = 5.0;
    final radius = size.width / 2 - stroke / 2;
    final rect = Rect.fromCircle(
      center: Offset(size.width / 2, size.height / 2),
      radius: radius,
    );
    final base = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = stroke
      ..strokeCap = StrokeCap.round
      ..color = track;
    canvas.drawArc(rect, 0, math.pi * 2, false, base);

    if (progress <= 0) {
      return;
    }
    final arc = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = stroke
      ..strokeCap = StrokeCap.round
      ..color = color;
    // Twelve o'clock, clockwise — the direction every arc in this app fills.
    canvas.drawArc(
      rect,
      -math.pi / 2,
      math.pi * 2 * progress.clamp(0.0, 1.0),
      false,
      arc,
    );
  }

  @override
  bool shouldRepaint(_GaugePainter oldDelegate) {
    return oldDelegate.progress != progress ||
        oldDelegate.color != color ||
        oldDelegate.track != track;
  }
}
