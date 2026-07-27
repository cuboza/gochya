import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../app/theme.dart';
import '../../core/models/activity_models.dart';
import '../activity/activity_repository.dart';
import '../activity/activity_rings.dart';
import '../activity/activity_screen.dart';

/// Symbiosis on the home screen: three activity rings for the owner's body and
/// the Vitality they earned, per `UX_UI.md` §7.1 and `ART_BIBLE.md` §6.3.
///
/// This replaces the plain entry card rather than sitting next to it. The home
/// screen is already a long column, and adding a fourth block would cost more
/// than the rings gain.
class SymbiosisCard extends ConsumerWidget {
  const SymbiosisCard({required this.accessToken, super.key});

  final String accessToken;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final week = ref.watch(activityWeekProvider(accessToken));
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              'Симбиоз',
              style: Theme.of(
                context,
              ).textTheme.titleLarge?.copyWith(fontWeight: FontWeight.w800),
            ),
            const SizedBox(height: 16),
            week.when(
              // A background refresh must not blank the rings the player is
              // already looking at.
              skipLoadingOnReload: true,
              loading: () => const _RingsRow(day: null, pending: true),
              error: (error, stackTrace) => const _Unavailable(
                message:
                    'Не удалось прочитать активность. Потяните экран, чтобы '
                    'повторить.',
              ),
              data: (days) => days.isEmpty
                  ? const _Unavailable(
                      message:
                          'Подключите источник здоровья — тогда шаги, сон и '
                          'тренировки начнут растить питомца.',
                    )
                  : _RingsRow(day: days.first, pending: false),
            ),
            const SizedBox(height: 18),
            FilledButton.tonalIcon(
              onPressed: () {
                Navigator.of(context).push(
                  MaterialPageRoute<void>(
                    builder: (context) =>
                        ActivityScreen(accessToken: accessToken),
                  ),
                );
              },
              icon: const Icon(Icons.monitor_heart_outlined),
              label: const Text('Активность и Vitality'),
            ),
          ],
        ),
      ),
    );
  }
}

class _RingsRow extends StatelessWidget {
  const _RingsRow({required this.day, required this.pending});

  final DailyActivity? day;
  final bool pending;

  @override
  Widget build(BuildContext context) {
    final current = day;
    final goals = current?.goals;
    final snapshot = current?.snapshot;

    final steps = RingProgress(
      value: (snapshot?.steps ?? 0).toDouble(),
      goal: (goals?.steps ?? 0).toDouble(),
      color: GochyaRingColors.steps,
    );
    final sleep = RingProgress(
      value: snapshot == null ? 0 : snapshot.sleepMinutes / 60,
      goal: goals?.sleepHours ?? 0,
      color: GochyaRingColors.sleep,
    );
    final calories = RingProgress(
      value: (snapshot?.activeCalories ?? 0).toDouble(),
      goal: (goals?.activeCalories ?? 0).toDouble(),
      color: GochyaRingColors.calories,
    );

    final rings = Semantics(
      label: current == null
          ? 'Кольца активности пока пусты'
          : 'Vitality ${current.vitality} из $maxVitalityPerDay. '
                'Шаги ${snapshot!.steps} из ${goals!.steps}. '
                'Сон ${_hours(snapshot.sleepMinutes / 60)} из '
                '${_hours(goals.sleepHours)} часов. '
                'Активные калории ${snapshot.activeCalories} из '
                '${goals.activeCalories}.',
      child: ExcludeSemantics(
        child: ActivityRings(
          steps: steps,
          sleep: sleep,
          calories: calories,
          centerLabel: current == null ? '—' : '${current.vitality}',
          centerCaption: 'Vitality',
        ),
      ),
    );

    Widget buildLegend({required bool stacked}) => Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        _LegendRow(
          icon: Icons.directions_walk,
          color: GochyaRingColors.steps,
          label: 'Шаги',
          stacked: stacked,
          value: current == null
              ? null
              : '${snapshot!.steps} / ${goals!.steps}',
          pending: pending,
        ),
        const SizedBox(height: 10),
        _LegendRow(
          icon: Icons.bedtime_outlined,
          color: GochyaRingColors.sleep,
          label: 'Сон',
          stacked: stacked,
          value: current == null
              ? null
              : '${_hours(snapshot!.sleepMinutes / 60)} / '
                    '${_hours(goals!.sleepHours)} ч',
          pending: pending,
        ),
        const SizedBox(height: 10),
        _LegendRow(
          icon: Icons.local_fire_department_outlined,
          color: GochyaRingColors.calories,
          label: 'Калории',
          stacked: stacked,
          value: current == null
              ? null
              : '${snapshot!.activeCalories} / ${goals!.activeCalories}',
          pending: pending,
        ),
      ],
    );

    // Two separate protections, because the legend runs out of room two ways.
    // A narrow phone is handled inside the row itself: caption and figure are
    // both flexible, so they wrap rather than overflow — that keeps the
    // side-by-side look on every real handset. Scaled-up type is different in
    // kind: wrapping every line makes an unreadable stack of fragments, so past
    // a moderate scale the rings move above the legend instead.
    final stacked = MediaQuery.textScalerOf(context).scale(14) > 20;
    if (stacked) {
      return Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Center(child: rings),
          const SizedBox(height: 18),
          buildLegend(stacked: true),
        ],
      );
    }
    return Row(
      crossAxisAlignment: CrossAxisAlignment.center,
      children: [
        rings,
        const SizedBox(width: _ringGap),
        Expanded(child: buildLegend(stacked: false)),
      ],
    );
  }

  static const _ringGap = 20.0;

  static String _hours(double value) => value.toStringAsFixed(1);
}

class _LegendRow extends StatelessWidget {
  const _LegendRow({
    required this.icon,
    required this.color,
    required this.label,
    required this.value,
    required this.pending,
    required this.stacked,
  });

  final IconData icon;
  final Color color;
  final String label;
  final String? value;
  final bool pending;
  final bool stacked;

  @override
  Widget build(BuildContext context) {
    final caption = Text(
      label,
      style: Theme.of(
        context,
      ).textTheme.bodyMedium?.copyWith(color: GochyaColors.muted),
    );
    final figure = Text(
      value ?? (pending ? '…' : '—'),
      style: Theme.of(context).textTheme.bodyMedium?.copyWith(
        fontWeight: FontWeight.w700,
        // Digits line up across the three rows instead of dancing.
        fontFeatures: const [FontFeature.tabularFigures()],
      ),
    );

    // Side by side the figure keeps its natural width and the caption takes
    // the rest; at large type there is no rest left, so they stack. Truncating
    // «11240 / 9000» was never an option — a goal you cannot read is no goal.
    if (stacked) {
      return Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Padding(
            padding: const EdgeInsets.only(top: 4),
            child: Icon(icon, size: 18, color: color),
          ),
          const SizedBox(width: 8),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [caption, figure],
            ),
          ),
        ],
      );
    }
    return Row(
      children: [
        Icon(icon, size: 18, color: color),
        const SizedBox(width: 8),
        // The caption absorbs the slack, the figure bends only when there is
        // none left: «Калории» wraps happily, «380 / 420» does not.
        Expanded(child: caption),
        const SizedBox(width: 8),
        Flexible(child: figure),
      ],
    );
  }
}

class _Unavailable extends StatelessWidget {
  const _Unavailable({required this.message});

  final String message;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const _RingsRow(day: null, pending: false),
        const SizedBox(height: 14),
        Text(
          message,
          style: Theme.of(
            context,
          ).textTheme.bodyMedium?.copyWith(color: GochyaColors.muted),
        ),
      ],
    );
  }
}
