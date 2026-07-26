import 'package:flutter/material.dart';

/// Below this the need demands attention (`ART_BIBLE.md` §5).
const int lowNeedThreshold = 30;

/// One need with its bar.
///
/// A need under [lowNeedThreshold] is called out. `ART_BIBLE.md` §5 asks for a
/// blink; this pulses slowly instead. A true blink is a flash, and WCAG 2.3.1
/// rules those out — the intent (pull the eye) is kept, the seizure risk is not.
///
/// The pulse is never the only signal. It stops entirely under reduced motion,
/// and a static «мало» marker carries the same meaning for anyone who cannot
/// see movement or colour.
///
/// The marker takes the need's own colour, never `warning`: §5 records that
/// low mood in the warning pink used to be indistinguishable from an error.
class NeedIndicator extends StatefulWidget {
  const NeedIndicator({
    required this.label,
    required this.value,
    required this.color,
    this.bottomPadding = 14,
    super.key,
  });

  final String label;
  final int value;
  final Color color;
  final double bottomPadding;

  bool get isLow => value < lowNeedThreshold;

  @override
  State<NeedIndicator> createState() => _NeedIndicatorState();
}

class _NeedIndicatorState extends State<NeedIndicator>
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

    return Padding(
      padding: EdgeInsets.only(bottom: widget.bottomPadding),
      child: Semantics(
        label: widget.isLow
            ? '${widget.label}: ${widget.value} из 100, мало'
            : '${widget.label}: ${widget.value} из 100',
        child: ExcludeSemantics(
          child: Column(
            children: [
              Row(
                children: [
                  Expanded(child: Text(widget.label)),
                  if (widget.isLow) ...[
                    Icon(
                      Icons.priority_high_rounded,
                      size: 15,
                      color: widget.color,
                    ),
                    Text(
                      'мало',
                      style: Theme.of(context).textTheme.labelMedium?.copyWith(
                        color: widget.color,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                    const SizedBox(width: 10),
                  ],
                  Text(
                    '${widget.value}%',
                    style: const TextStyle(fontWeight: FontWeight.w700),
                  ),
                ],
              ),
              const SizedBox(height: 7),
              AnimatedBuilder(
                animation: _pulse,
                builder: (context, child) {
                  // Never fades far enough to become unreadable.
                  final alpha = _running ? 1 - _pulse.value * 0.45 : 1.0;
                  return Opacity(opacity: alpha, child: child);
                },
                child: LinearProgressIndicator(
                  value: widget.value / 100,
                  minHeight: 9,
                  borderRadius: BorderRadius.circular(10),
                  color: widget.color,
                  backgroundColor: widget.color.withValues(alpha: 0.14),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
