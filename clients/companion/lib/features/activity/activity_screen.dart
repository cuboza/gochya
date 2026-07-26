import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../app/theme.dart';
import '../../core/models/activity_models.dart';
import '../../core/network/gochya_api_client.dart';
import '../techniques/technique_content.dart';
import '../techniques/technique_repository.dart';
import 'activity_repository.dart';

/// Read-only view of the server-side symbiosis ledger plus the daily card
/// claim. Health data itself is ingested by the platform sync slice, never
/// invented here: the phone must not author activity numbers.
class ActivityScreen extends ConsumerStatefulWidget {
  const ActivityScreen({required this.accessToken, super.key});

  final String accessToken;

  @override
  ConsumerState<ActivityScreen> createState() => _ActivityScreenState();
}

class _ActivityScreenState extends ConsumerState<ActivityScreen> {
  ActivityRewardResult? _reward;
  String? _claimError;
  var _isClaiming = false;

  @override
  Widget build(BuildContext context) {
    final week = ref.watch(activityWeekProvider(widget.accessToken));
    return Scaffold(
      appBar: AppBar(title: const Text('Активность')),
      body: week.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (error, stackTrace) => _ActivityError(
          message: _loadMessage(error),
          onRetry: () =>
              ref.invalidate(activityWeekProvider(widget.accessToken)),
        ),
        data: (days) => RefreshIndicator(
          onRefresh: () =>
              ref.refresh(activityWeekProvider(widget.accessToken).future),
          child: ListView(
            physics: const AlwaysScrollableScrollPhysics(),
            padding: const EdgeInsets.fromLTRB(20, 12, 20, 32),
            children: [
              _TodayCard(
                today: days.isEmpty ? null : days.first,
                reward: _reward,
                claimError: _claimError,
                isClaiming: _isClaiming,
                onClaim: _claim,
              ),
              const SizedBox(height: 24),
              Text(
                'Неделя',
                style: Theme.of(
                  context,
                ).textTheme.titleLarge?.copyWith(fontWeight: FontWeight.w800),
              ),
              const SizedBox(height: 12),
              if (days.isEmpty)
                const _NoActivityCard()
              else
                for (final day in days) ...[
                  _DayCard(day: day),
                  const SizedBox(height: 10),
                ],
            ],
          ),
        ),
      ),
    );
  }

  Future<void> _claim() async {
    setState(() {
      _isClaiming = true;
      _claimError = null;
    });
    try {
      final reward = await ref
          .read(activityRepositoryProvider)
          .claimReward(widget.accessToken);
      if (!mounted) {
        return;
      }
      setState(() {
        _reward = reward;
        _isClaiming = false;
      });
      ref.invalidate(loadoutSnapshotProvider(widget.accessToken));
    } on Object catch (error) {
      if (!mounted) {
        return;
      }
      setState(() {
        _isClaiming = false;
        _claimError = _claimMessage(error);
      });
    }
  }

  String _claimMessage(Object error) {
    if (error is! ApiException) {
      return 'Награда не подтверждена. Повтори — выдача идемпотентная.';
    }
    return switch (error.code) {
      'activity_reward_locked' =>
        'Нужно $activityRewardVitality Vitality за день.',
      'activity_required' => 'Сегодняшняя активность ещё не синхронизирована.',
      'pet_state_invalid' =>
        'Состояние активного питомца не позволяет награду.',
      'profile_not_found' => 'Сначала заведи питомца.',
      'core_unavailable' => 'Ядро недоступно. Повтори позже.',
      _ => 'Награда не подтверждена. Повтори — выдача идемпотентная.',
    };
  }

  String _loadMessage(Object error) {
    if (error is ApiException &&
        (error.code == 'request_timeout' || error.code == 'network_error')) {
      return 'Vitality хранится на сервере. Проверь соединение.';
    }
    return 'Не удалось загрузить неделю активности.';
  }
}

class _TodayCard extends StatelessWidget {
  const _TodayCard({
    required this.today,
    required this.reward,
    required this.claimError,
    required this.isClaiming,
    required this.onClaim,
  });

  final DailyActivity? today;
  final ActivityRewardResult? reward;
  final String? claimError;
  final bool isClaiming;
  final Future<void> Function() onClaim;

  @override
  Widget build(BuildContext context) {
    final day = today;
    final unlocked = day?.unlocksReward ?? false;
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              'Сегодня',
              style: Theme.of(
                context,
              ).textTheme.titleLarge?.copyWith(fontWeight: FontWeight.w800),
            ),
            const SizedBox(height: 12),
            if (day == null)
              const Text(
                'За сегодня активность ещё не синхронизирована. Vitality '
                'начисляется только по данным Health Connect / HealthKit.',
                style: TextStyle(color: GochyaColors.muted),
              )
            else ...[
              Row(
                children: [
                  Expanded(
                    child: Text(
                      'Vitality ${day.vitality} '
                      'из $activityRewardVitality до карты',
                    ),
                  ),
                  Text(
                    '${(day.rewardProgress * 100).round()}%',
                    style: const TextStyle(fontWeight: FontWeight.w800),
                  ),
                ],
              ),
              const SizedBox(height: 8),
              LinearProgressIndicator(
                value: day.rewardProgress,
                minHeight: 9,
                borderRadius: BorderRadius.circular(10),
                color: GochyaColors.success,
                backgroundColor: GochyaColors.success.withValues(alpha: 0.14),
              ),
              const SizedBox(height: 12),
              Text(
                '${day.snapshot.steps} шагов · '
                'сон ${day.snapshot.sleepHours.toStringAsFixed(1)} ч · '
                'тренировок ${day.snapshot.workouts}',
                style: const TextStyle(color: GochyaColors.muted),
              ),
              const SizedBox(height: 4),
              Text(
                'Статы за день: +${day.statGains.total}',
                style: const TextStyle(color: GochyaColors.muted),
              ),
            ],
            if (reward != null) ...[
              const SizedBox(height: 14),
              Row(
                children: [
                  Icon(
                    Icons.auto_awesome_rounded,
                    color: reward!.card.rarity.frameColor,
                  ),
                  const SizedBox(width: 8),
                  Expanded(
                    child: Text(
                      reward!.awarded
                          ? 'Карта дня: ${reward!.card.label} · '
                                '${reward!.card.rarity.label}'
                          : 'Карта дня уже была получена: '
                                '${reward!.card.label}',
                      style: const TextStyle(fontWeight: FontWeight.w700),
                    ),
                  ),
                ],
              ),
            ],
            if (claimError != null) ...[
              const SizedBox(height: 12),
              Text(
                claimError!,
                style: TextStyle(color: Theme.of(context).colorScheme.error),
              ),
            ],
            const SizedBox(height: 16),
            FilledButton.icon(
              onPressed: !unlocked || isClaiming || reward != null
                  ? null
                  : onClaim,
              icon: isClaiming
                  ? const SizedBox.square(
                      dimension: 18,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    )
                  : const Icon(Icons.card_giftcard_rounded),
              label: const Text('Забрать карту дня'),
            ),
          ],
        ),
      ),
    );
  }
}

class _DayCard extends StatelessWidget {
  const _DayCard({required this.day});

  final DailyActivity day;

  @override
  Widget build(BuildContext context) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Row(
          children: [
            SizedBox(
              width: 78,
              child: Text(
                day.date,
                style: const TextStyle(color: GochyaColors.muted, fontSize: 12),
              ),
            ),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    'Vitality ${day.vitality} · ${day.snapshot.steps} шагов',
                    style: const TextStyle(fontWeight: FontWeight.w700),
                  ),
                  const SizedBox(height: 4),
                  LinearProgressIndicator(
                    value: day.vitality / maxVitalityPerDay,
                    minHeight: 6,
                    borderRadius: BorderRadius.circular(8),
                    color: day.unlocksReward
                        ? GochyaColors.success
                        : GochyaColors.energy,
                    backgroundColor: GochyaColors.muted.withValues(alpha: 0.2),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _NoActivityCard extends StatelessWidget {
  const _NoActivityCard();

  @override
  Widget build(BuildContext context) {
    return const Card(
      child: Padding(
        padding: EdgeInsets.all(24),
        child: Column(
          children: [
            Icon(Icons.directions_walk_rounded, size: 48),
            SizedBox(height: 10),
            Text(
              'Данных за неделю нет',
              style: TextStyle(fontWeight: FontWeight.w800),
            ),
            SizedBox(height: 6),
            Text(
              'Симбиоз включается после подключения источника здоровья. '
              'Сырые сигналы сенсоров не покидают устройство.',
              textAlign: TextAlign.center,
              style: TextStyle(color: GochyaColors.muted),
            ),
          ],
        ),
      ),
    );
  }
}

class _ActivityError extends StatelessWidget {
  const _ActivityError({required this.message, required this.onRetry});

  final String message;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.monitor_heart_outlined, size: 64),
            const SizedBox(height: 16),
            Text(message, textAlign: TextAlign.center),
            const SizedBox(height: 16),
            FilledButton.icon(
              onPressed: onRetry,
              icon: const Icon(Icons.refresh_rounded),
              label: const Text('Повторить'),
            ),
          ],
        ),
      ),
    );
  }
}
