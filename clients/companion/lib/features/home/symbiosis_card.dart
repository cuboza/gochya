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
    // Rings, Vitality and a tap. The exact figures moved to the activity
    // screen: on the main screen the ring already answers "am I on track",
    // and the numbers only repeated it while costing a third of the card.
    return Card(
      child: InkWell(
        borderRadius: BorderRadius.circular(20),
        onTap: () {
          Navigator.of(context).push(
            MaterialPageRoute<void>(
              builder: (context) => ActivityScreen(accessToken: accessToken),
            ),
          );
        },
        child: Padding(
          padding: const EdgeInsets.all(20),
          child: week.when(
            // A background refresh must not blank the rings the player is
            // already looking at.
            skipLoadingOnReload: true,
            loading: () => const _Summary(day: null),
            error: (error, stackTrace) => const _Summary(
              day: null,
              message: 'Не удалось прочитать активность. Потяните экран.',
            ),
            data: (days) => days.isEmpty
                ? const _Summary(
                    day: null,
                    message:
                        'Подключите источник здоровья — шаги, сон и '
                        'тренировки начнут растить питомца.',
                  )
                : _Summary(day: days.first),
          ),
        ),
      ),
    );
  }
}

class _Summary extends StatelessWidget {
  const _Summary({required this.day, this.message});

  final DailyActivity? day;
  final String? message;

  @override
  Widget build(BuildContext context) {
    final current = day;
    final goals = current?.goals;
    final snapshot = current?.snapshot;
    final note = message;

    return Row(
      children: [
        Semantics(
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
              steps: RingProgress(
                value: (snapshot?.steps ?? 0).toDouble(),
                goal: (goals?.steps ?? 0).toDouble(),
                color: GochyaRingColors.steps,
              ),
              sleep: RingProgress(
                value: snapshot == null ? 0 : snapshot.sleepMinutes / 60,
                goal: goals?.sleepHours ?? 0,
                color: GochyaRingColors.sleep,
              ),
              calories: RingProgress(
                value: (snapshot?.activeCalories ?? 0).toDouble(),
                goal: (goals?.activeCalories ?? 0).toDouble(),
                color: GochyaRingColors.calories,
              ),
              centerLabel: current == null ? '—' : '${current.vitality}',
              centerCaption: 'Vitality',
            ),
          ),
        ),
        const SizedBox(width: 18),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                'Симбиоз',
                style: Theme.of(
                  context,
                ).textTheme.titleLarge?.copyWith(fontWeight: FontWeight.w800),
              ),
              const SizedBox(height: 6),
              Text(
                note ??
                    'Шаги, сон и калории за сегодня. Нажмите, чтобы увидеть '
                        'неделю и забрать карту дня.',
                style: Theme.of(
                  context,
                ).textTheme.bodyMedium?.copyWith(color: GochyaColors.muted),
              ),
            ],
          ),
        ),
      ],
    );
  }

  static String _hours(double value) => value.toStringAsFixed(1);
}
