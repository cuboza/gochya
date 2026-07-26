import 'dart:math';

import 'package:flutter/material.dart';

import '../../core/models/technique_models.dart';

/// A motion signature drawn per strike type, so every technique reads as a
/// different move at a glance instead of sharing one generic icon.
///
/// Each glyph is the path the strike travels: straight for a jab, hooking for
/// a hook, rising for an uppercut, sweeping for a kick, folded for an elbow,
/// closed for a block.
class TechniqueGlyph extends StatelessWidget {
  const TechniqueGlyph({
    required this.type,
    required this.color,
    required this.size,
    super.key,
  });

  final TechniqueType type;
  final Color color;
  final double size;

  @override
  Widget build(BuildContext context) {
    return SizedBox.square(
      dimension: size,
      child: CustomPaint(
        painter: _GlyphPainter(type: type, color: color),
        isComplex: false,
        willChange: false,
      ),
    );
  }
}

class _GlyphPainter extends CustomPainter {
  const _GlyphPainter({required this.type, required this.color});

  final TechniqueType type;
  final Color color;

  @override
  void paint(Canvas canvas, Size size) {
    final unit = size.shortestSide;
    final stroke = Paint()
      ..color = color
      ..style = PaintingStyle.stroke
      ..strokeCap = StrokeCap.round
      ..strokeJoin = StrokeJoin.round
      ..strokeWidth = unit * 0.075;
    final ghost = Paint()
      ..color = color.withValues(alpha: 0.35)
      ..style = PaintingStyle.stroke
      ..strokeCap = StrokeCap.round
      ..strokeWidth = unit * 0.045;

    Offset at(double x, double y) => Offset(x * size.width, y * size.height);

    switch (type) {
      case TechniqueType.jab:
        // Short, straight, repeated: the fastest line between two points.
        canvas.drawLine(at(0.24, 0.5), at(0.72, 0.5), stroke);
        _arrow(canvas, at(0.72, 0.5), 0, unit, stroke);
        canvas.drawLine(at(0.16, 0.34), at(0.36, 0.34), ghost);
        canvas.drawLine(at(0.16, 0.66), at(0.36, 0.66), ghost);
      case TechniqueType.hook:
        // Comes around the guard instead of through it.
        final path = Path()
          ..moveTo(size.width * 0.2, size.height * 0.26)
          ..quadraticBezierTo(
            size.width * 0.88,
            size.height * 0.32,
            size.width * 0.6,
            size.height * 0.76,
          );
        canvas.drawPath(path, stroke);
        _arrow(canvas, at(0.6, 0.76), pi * 0.72, unit, stroke);
      case TechniqueType.uppercut:
        // Rises out of the legs, through the body, into the chin.
        final path = Path()
          ..moveTo(size.width * 0.3, size.height * 0.84)
          ..quadraticBezierTo(
            size.width * 0.34,
            size.height * 0.36,
            size.width * 0.7,
            size.height * 0.22,
          );
        canvas.drawPath(path, stroke);
        _arrow(canvas, at(0.7, 0.22), -pi * 0.18, unit, stroke);
        canvas.drawLine(at(0.2, 0.86), at(0.44, 0.86), ghost);
      case TechniqueType.cross:
        // Long diagonal with the hip rotation that powers it.
        canvas.drawLine(at(0.16, 0.74), at(0.82, 0.28), stroke);
        _arrow(canvas, at(0.82, 0.28), -pi * 0.19, unit, stroke);
        final twist = Path()
          ..addArc(
            Rect.fromCircle(center: at(0.22, 0.72), radius: unit * 0.16),
            pi * 0.25,
            pi * 1.1,
          );
        canvas.drawPath(twist, ghost);
      case TechniqueType.kick:
        // Widest arc, most mass, longest travel.
        final path = Path()
          ..moveTo(size.width * 0.16, size.height * 0.84)
          ..quadraticBezierTo(
            size.width * 0.28,
            size.height * 0.24,
            size.width * 0.84,
            size.height * 0.5,
          );
        canvas.drawPath(path, stroke..strokeWidth = unit * 0.09);
        _arrow(canvas, at(0.84, 0.5), pi * 0.16, unit, stroke);
      case TechniqueType.elbow:
        // Folded limb: a blade at breathing distance.
        final path = Path()
          ..moveTo(size.width * 0.22, size.height * 0.28)
          ..lineTo(size.width * 0.66, size.height * 0.54)
          ..lineTo(size.width * 0.3, size.height * 0.8);
        canvas.drawPath(path, stroke);
        canvas.drawCircle(at(0.66, 0.54), unit * 0.07, Paint()..color = color);
      case TechniqueType.block:
        // Closed shape: nothing gets through.
        final shield = Path()
          ..moveTo(size.width * 0.5, size.height * 0.18)
          ..lineTo(size.width * 0.78, size.height * 0.32)
          ..lineTo(size.width * 0.78, size.height * 0.56)
          ..quadraticBezierTo(
            size.width * 0.78,
            size.height * 0.76,
            size.width * 0.5,
            size.height * 0.86,
          )
          ..quadraticBezierTo(
            size.width * 0.22,
            size.height * 0.76,
            size.width * 0.22,
            size.height * 0.56,
          )
          ..lineTo(size.width * 0.22, size.height * 0.32)
          ..close();
        canvas.drawPath(shield, stroke);
    }
  }

  /// Arrowhead pointing along [angle], drawn at [tip].
  void _arrow(
    Canvas canvas,
    Offset tip,
    double angle,
    double unit,
    Paint paint,
  ) {
    const spread = pi * 0.82;
    final length = unit * 0.2;
    final left = tip + Offset.fromDirection(angle + spread, length);
    final right = tip + Offset.fromDirection(angle - spread, length);
    canvas.drawLine(tip, left, paint);
    canvas.drawLine(tip, right, paint);
  }

  @override
  bool shouldRepaint(_GlyphPainter oldDelegate) =>
      oldDelegate.type != type || oldDelegate.color != color;
}
