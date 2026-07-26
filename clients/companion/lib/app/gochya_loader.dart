import 'package:flutter/material.dart';

import 'theme.dart';

/// Branded screen-level loading state, per `UX_UI.md` §8 (audit V8): a bare
/// spinner is called out there by name.
///
/// Four dots in the needs palette — hunger, energy, hygiene, mood — because
/// those four colours *are* the game's visual language. A creature silhouette
/// would read better still, but the only creature art we ship is per-element,
/// and a screen that has not loaded yet does not know the pet's species;
/// picking one would promise a creature the player may not own.
///
/// Under reduced motion the dots stand still rather than disappearing: the
/// point is to say "something is coming", which a static row still does.
class GochyaLoader extends StatefulWidget {
  const GochyaLoader({this.caption, super.key});

  /// What is being waited for. Keep it concrete — "Ищем соперника" beats
  /// "Загрузка".
  final String? caption;

  @override
  State<GochyaLoader> createState() => _GochyaLoaderState();
}

class _GochyaLoaderState extends State<GochyaLoader>
    with SingleTickerProviderStateMixin {
  static const _colors = [
    GochyaColors.hunger,
    GochyaColors.energy,
    GochyaColors.hygiene,
    GochyaColors.mood,
  ];

  late final AnimationController _controller = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 1400),
  );

  bool _running = false;

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  void _sync({required bool wanted}) {
    if (wanted == _running) {
      return;
    }
    _running = wanted;
    if (wanted) {
      _controller.repeat();
    } else {
      _controller.stop();
      _controller.value = 0;
    }
  }

  @override
  Widget build(BuildContext context) {
    _sync(wanted: !MediaQuery.disableAnimationsOf(context));
    final caption = widget.caption;

    return Semantics(
      label: caption ?? 'Загрузка',
      child: ExcludeSemantics(
        child: Center(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              AnimatedBuilder(
                animation: _controller,
                builder: (context, _) {
                  return Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      for (var index = 0; index < _colors.length; index += 1)
                        Padding(
                          padding: const EdgeInsets.symmetric(horizontal: 5),
                          child: _Dot(
                            color: _colors[index],
                            scale: _running
                                ? _scaleFor(index, _controller.value)
                                : 1,
                          ),
                        ),
                    ],
                  );
                },
              ),
              if (caption != null) ...[
                const SizedBox(height: 18),
                Text(
                  caption,
                  textAlign: TextAlign.center,
                  style: Theme.of(
                    context,
                  ).textTheme.bodyMedium?.copyWith(color: GochyaColors.muted),
                ),
              ],
            ],
          ),
        ),
      ),
    );
  }

  /// Each dot peaks a quarter-cycle after the one before it, so the row reads
  /// as a wave rather than four things blinking at once.
  static double _scaleFor(int index, double t) {
    final phase = (t - index * 0.16) % 1.0;
    if (phase > 0.5) {
      return 0.72;
    }
    // 0 -> 1 -> 0 across the first half of the cycle.
    final wave = 1 - (phase * 4 - 1).abs();
    return 0.72 + wave.clamp(0.0, 1.0) * 0.48;
  }
}

class _Dot extends StatelessWidget {
  const _Dot({required this.color, required this.scale});

  final Color color;
  final double scale;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: 14,
      height: 14,
      child: Center(
        child: Container(
          width: 12 * scale,
          height: 12 * scale,
          decoration: BoxDecoration(color: color, shape: BoxShape.circle),
        ),
      ),
    );
  }
}
