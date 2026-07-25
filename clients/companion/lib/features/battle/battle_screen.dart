import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../app/theme.dart';
import '../../core/identifiers/uuid_v4.dart';
import '../../core/models/battle_models.dart';
import '../../core/models/technique_models.dart';
import '../../core/network/gochya_api_client.dart';
import '../home/profile_repository.dart';
import '../techniques/loadout_screen.dart';
import '../techniques/technique_repository.dart';
import 'battle_repository.dart';

/// Casual PvP. The server queues, simulates through Rust Core and confirms;
/// the phone only submits intent and renders the authoritative replay.
class BattleScreen extends ConsumerStatefulWidget {
  const BattleScreen({required this.accessToken, super.key});

  final String accessToken;

  @override
  ConsumerState<BattleScreen> createState() => _BattleScreenState();
}

class _BattleScreenState extends ConsumerState<BattleScreen> {
  MatchReplay? _replay;
  MatchConfirmation? _confirmation;
  String? _queueIdempotencyKey;
  String? _actionError;
  var _isQueueing = false;
  var _isConfirming = false;

  @override
  Widget build(BuildContext context) {
    final home = ref.watch(homeSnapshotProvider(widget.accessToken));
    final loadout = ref.watch(loadoutSnapshotProvider(widget.accessToken));
    return Scaffold(
      appBar: AppBar(title: const Text('PvP')),
      body: RefreshIndicator(
        onRefresh: () {
          ref.invalidate(loadoutSnapshotProvider(widget.accessToken));
          ref.invalidate(matchHistoryProvider(widget.accessToken));
          return ref.refresh(homeSnapshotProvider(widget.accessToken).future);
        },
        child: ListView(
          physics: const AlwaysScrollableScrollPhysics(),
          padding: const EdgeInsets.fromLTRB(20, 12, 20, 32),
          children: [
            loadout.when(
              loading: () => const Card(
                child: Padding(
                  padding: EdgeInsets.all(24),
                  child: Center(child: CircularProgressIndicator()),
                ),
              ),
              error: (error, stackTrace) => const _MessageCard(
                icon: Icons.cloud_off_outlined,
                title: 'Лоадаут недоступен',
                message: 'Бой требует серверный лоадаут. Проверь соединение.',
              ),
              data: (value) =>
                  _ReadinessCard(snapshot: value, onOpenLoadout: _openLoadout),
            ),
            const SizedBox(height: 16),
            _QueueCard(
              isReady: loadout.value?.isBattleReady ?? false,
              isQueueing: _isQueueing,
              hasPendingRetry: _queueIdempotencyKey != null,
              error: _actionError,
              onQueue: _queue,
            ),
            if (_replay case final replay?) ...[
              const SizedBox(height: 16),
              _ReplayCard(
                replay: replay,
                playerId: home.value?.profile.id,
                confirmation: _confirmation,
                isConfirming: _isConfirming,
                onConfirm: _confirm,
              ),
            ],
            const SizedBox(height: 24),
            Text(
              'История боёв',
              style: Theme.of(
                context,
              ).textTheme.titleLarge?.copyWith(fontWeight: FontWeight.w800),
            ),
            const SizedBox(height: 12),
            _HistorySection(accessToken: widget.accessToken),
          ],
        ),
      ),
    );
  }

  void _openLoadout() {
    Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (context) => LoadoutScreen(accessToken: widget.accessToken),
      ),
    );
  }

  Future<void> _queue() async {
    // Reusing the key after an uncertain queue response makes the server
    // return the original match instead of starting a second one.
    final idempotencyKey = _queueIdempotencyKey ?? newUuidV4();
    setState(() {
      _isQueueing = true;
      _queueIdempotencyKey = idempotencyKey;
      _actionError = null;
      _confirmation = null;
    });
    try {
      final replay = await ref
          .read(battleRepositoryProvider)
          .queueCasual(
            accessToken: widget.accessToken,
            idempotencyKey: idempotencyKey,
          );
      if (!mounted) {
        return;
      }
      setState(() {
        _replay = replay;
        _isQueueing = false;
        _queueIdempotencyKey = null;
      });
      ref.invalidate(matchHistoryProvider(widget.accessToken));
    } on Object catch (error) {
      if (!mounted) {
        return;
      }
      setState(() {
        _isQueueing = false;
        _actionError = _queueMessage(error);
      });
    }
  }

  Future<void> _confirm() async {
    final replay = _replay;
    if (replay == null) {
      return;
    }
    setState(() {
      _isConfirming = true;
      _actionError = null;
    });
    try {
      final confirmation = await ref
          .read(battleRepositoryProvider)
          .confirm(accessToken: widget.accessToken, matchId: replay.id);
      if (!mounted) {
        return;
      }
      setState(() {
        _confirmation = confirmation;
        _isConfirming = false;
      });
      ref.invalidate(matchHistoryProvider(widget.accessToken));
      if (confirmation.card != null) {
        ref.invalidate(loadoutSnapshotProvider(widget.accessToken));
      }
    } on Object catch (error) {
      if (!mounted) {
        return;
      }
      setState(() {
        _isConfirming = false;
        _actionError = _confirmMessage(error);
      });
    }
  }

  String _queueMessage(Object error) {
    if (error is! ApiException) {
      return 'Исход неизвестен. Повтори — тот же ключ вернёт тот же бой.';
    }
    return switch (error.code) {
      'loadout_required' => 'Собери лоадаут из пяти карт перед боем.',
      'pet_weak' => 'Ослабленный питомец не выходит в бой. Сначала уход.',
      'no_opponent' => 'Подходящий соперник пока не найден. Попробуй позже.',
      'profile_not_found' => 'Сначала заведи питомца.',
      'core_unavailable' => 'Боевое ядро недоступно. Повтори позже.',
      'idempotency_conflict' => 'Ключ уже использован другим запросом.',
      'request_timeout' || 'network_error' =>
        'Исход неизвестен. Повтори — тот же ключ вернёт тот же бой.',
      _ => 'Не удалось начать бой.',
    };
  }

  String _confirmMessage(Object error) {
    if (error is ApiException && error.code == 'match_not_found') {
      return 'Бой не найден. Обнови историю.';
    }
    return 'Награда не подтверждена. Повтори — подтверждение идемпотентное.';
  }
}

class _ReadinessCard extends StatelessWidget {
  const _ReadinessCard({required this.snapshot, required this.onOpenLoadout});

  final LoadoutSnapshot snapshot;
  final VoidCallback onOpenLoadout;

  @override
  Widget build(BuildContext context) {
    final loadout = snapshot.loadout;
    final equipped = snapshot.equippedCards;
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(
                  loadout == null
                      ? Icons.shield_outlined
                      : Icons.verified_user_rounded,
                  color: loadout == null
                      ? GochyaColors.warning
                      : GochyaColors.success,
                ),
                const SizedBox(width: 10),
                Expanded(
                  child: Text(
                    loadout == null
                        ? 'Лоадаут не собран'
                        : 'К бою готов · ревизия ${loadout.revision}',
                    style: Theme.of(context).textTheme.titleMedium?.copyWith(
                      fontWeight: FontWeight.w800,
                    ),
                  ),
                ),
              ],
            ),
            if (equipped.isNotEmpty) ...[
              const SizedBox(height: 12),
              Wrap(
                spacing: 8,
                runSpacing: 8,
                children: [
                  for (var index = 0; index < equipped.length; index++)
                    Chip(
                      avatar: Icon(
                        index == loadout?.signatureIdx
                            ? Icons.star_rounded
                            : Icons.circle,
                        size: 16,
                        color: equipped[index].rarity.color,
                      ),
                      label: Text(equipped[index].type.label),
                    ),
                ],
              ),
            ],
            const SizedBox(height: 14),
            FilledButton.tonalIcon(
              onPressed: onOpenLoadout,
              icon: const Icon(Icons.style_outlined),
              label: Text(loadout == null ? 'Собрать лоадаут' : 'Изменить'),
            ),
          ],
        ),
      ),
    );
  }
}

class _QueueCard extends StatelessWidget {
  const _QueueCard({
    required this.isReady,
    required this.isQueueing,
    required this.hasPendingRetry,
    required this.error,
    required this.onQueue,
  });

  final bool isReady;
  final bool isQueueing;
  final bool hasPendingRetry;
  final String? error;
  final Future<void> Function() onQueue;

  @override
  Widget build(BuildContext context) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          children: [
            const Icon(Icons.sports_martial_arts_rounded, size: 44),
            const SizedBox(height: 10),
            Text(
              'Casual-бой',
              style: Theme.of(
                context,
              ).textTheme.titleLarge?.copyWith(fontWeight: FontWeight.w800),
            ),
            const SizedBox(height: 6),
            const Text(
              'Единый кроссплатформенный пул. Исход считает сервер по двум '
              'авторитетным лоадаутам.',
              textAlign: TextAlign.center,
              style: TextStyle(color: GochyaColors.muted),
            ),
            if (error != null) ...[
              const SizedBox(height: 12),
              Text(
                error!,
                textAlign: TextAlign.center,
                style: TextStyle(color: Theme.of(context).colorScheme.error),
              ),
            ],
            const SizedBox(height: 16),
            FilledButton.icon(
              onPressed: !isReady || isQueueing ? null : onQueue,
              icon: isQueueing
                  ? const SizedBox.square(
                      dimension: 18,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    )
                  : const Icon(Icons.play_arrow_rounded),
              label: Text(hasPendingRetry ? 'Повторить поиск' : 'Найти бой'),
            ),
          ],
        ),
      ),
    );
  }
}

class _ReplayCard extends StatelessWidget {
  const _ReplayCard({
    required this.replay,
    required this.playerId,
    required this.confirmation,
    required this.isConfirming,
    required this.onConfirm,
  });

  final MatchReplay replay;
  final String? playerId;
  final MatchConfirmation? confirmation;
  final bool isConfirming;
  final Future<void> Function() onConfirm;

  @override
  Widget build(BuildContext context) {
    final outcome = playerId == null ? null : replay.outcomeFor(playerId!);
    final reward = confirmation;
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(
                  switch (outcome) {
                    MatchOutcome.win => Icons.emoji_events_rounded,
                    MatchOutcome.loss => Icons.sentiment_dissatisfied_rounded,
                    MatchOutcome.draw => Icons.handshake_rounded,
                    null => Icons.sports_kabaddi_rounded,
                  },
                  color: switch (outcome) {
                    MatchOutcome.win => GochyaColors.success,
                    MatchOutcome.loss => GochyaColors.warning,
                    _ => GochyaColors.secondary,
                  },
                ),
                const SizedBox(width: 10),
                Expanded(
                  child: Text(
                    outcome?.label ?? 'Бой завершён',
                    style: Theme.of(context).textTheme.titleLarge?.copyWith(
                      fontWeight: FontWeight.w800,
                    ),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 8),
            if (playerId != null)
              Text(
                'HP ${replay.ownHp(playerId!)} : '
                '${replay.opponentHp(playerId!)} · '
                'раундов ${replay.rounds.length}',
                style: const TextStyle(color: GochyaColors.muted),
              ),
            const SizedBox(height: 14),
            for (var index = 0; index < replay.rounds.length; index++)
              _RoundRow(
                index: index,
                round: replay.rounds[index],
                isPlayerA: playerId == null || replay.isPlayerA(playerId!),
              ),
            const SizedBox(height: 16),
            if (reward == null)
              FilledButton.icon(
                onPressed: isConfirming ? null : onConfirm,
                icon: isConfirming
                    ? const SizedBox.square(
                        dimension: 18,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      )
                    : const Icon(Icons.redeem_rounded),
                label: const Text('Забрать награду'),
              )
            else
              _RewardSummary(confirmation: reward),
          ],
        ),
      ),
    );
  }
}

class _RoundRow extends StatelessWidget {
  const _RoundRow({
    required this.index,
    required this.round,
    required this.isPlayerA,
  });

  final int index;
  final MatchRound round;
  final bool isPlayerA;

  @override
  Widget build(BuildContext context) {
    final dealt = isPlayerA ? round.damageAToB : round.damageBToA;
    final taken = isPlayerA ? round.damageBToA : round.damageAToB;
    return Padding(
      padding: const EdgeInsets.only(bottom: 8),
      child: Row(
        children: [
          SizedBox(
            width: 34,
            child: Text(
              '${index + 1}',
              style: const TextStyle(color: GochyaColors.muted),
            ),
          ),
          Expanded(
            child: Text(
              'Нанесено $dealt · получено $taken'
              '${round.effect == TechniqueEffect.none ? '' : ' · ${round.effect.label}'}',
            ),
          ),
        ],
      ),
    );
  }
}

class _RewardSummary extends StatelessWidget {
  const _RewardSummary({required this.confirmation});

  final MatchConfirmation confirmation;

  @override
  Widget build(BuildContext context) {
    final card = confirmation.card;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            const Icon(
              Icons.monetization_on_rounded,
              color: GochyaColors.secondary,
            ),
            const SizedBox(width: 8),
            Text(
              '+${confirmation.koins} Koins · ${confirmation.outcome.label}',
              style: const TextStyle(fontWeight: FontWeight.w800),
            ),
          ],
        ),
        if (card != null) ...[
          const SizedBox(height: 10),
          Row(
            children: [
              Icon(Icons.auto_awesome_rounded, color: card.rarity.color),
              const SizedBox(width: 8),
              Expanded(
                child: Text(
                  'Новая карта: ${card.label} · ${card.rarity.label}',
                  style: const TextStyle(fontWeight: FontWeight.w700),
                ),
              ),
            ],
          ),
        ],
      ],
    );
  }
}

class _HistorySection extends ConsumerWidget {
  const _HistorySection({required this.accessToken});

  final String accessToken;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final history = ref.watch(matchHistoryProvider(accessToken));
    return history.when(
      loading: () => const Card(
        child: Padding(
          padding: EdgeInsets.all(24),
          child: Center(child: CircularProgressIndicator()),
        ),
      ),
      error: (error, stackTrace) => const _MessageCard(
        icon: Icons.history_toggle_off_rounded,
        title: 'История недоступна',
        message: 'Потяни экран вниз, чтобы обновить.',
      ),
      data: (values) {
        if (values.isEmpty) {
          return const _MessageCard(
            icon: Icons.history_rounded,
            title: 'Боёв ещё не было',
            message: 'Первая победа дня приносит Technique Card.',
          );
        }
        return Column(
          children: [
            for (final match in values) ...[
              Card(
                child: ListTile(
                  leading: Icon(
                    switch (match.outcome) {
                      MatchOutcome.win => Icons.emoji_events_outlined,
                      MatchOutcome.loss => Icons.close_rounded,
                      MatchOutcome.draw => Icons.remove_rounded,
                    },
                    color: switch (match.outcome) {
                      MatchOutcome.win => GochyaColors.success,
                      MatchOutcome.loss => GochyaColors.warning,
                      MatchOutcome.draw => GochyaColors.muted,
                    },
                  ),
                  title: Text(match.outcome.label),
                  subtitle: Text(
                    '${match.mode} · ${_shortDate(match.createdAt)}',
                  ),
                ),
              ),
              const SizedBox(height: 8),
            ],
          ],
        );
      },
    );
  }
}

class _MessageCard extends StatelessWidget {
  const _MessageCard({
    required this.icon,
    required this.title,
    required this.message,
  });

  final IconData icon;
  final String title;
  final String message;

  @override
  Widget build(BuildContext context) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          children: [
            Icon(icon, size: 44),
            const SizedBox(height: 10),
            Text(title, style: const TextStyle(fontWeight: FontWeight.w800)),
            const SizedBox(height: 6),
            Text(
              message,
              textAlign: TextAlign.center,
              style: const TextStyle(color: GochyaColors.muted),
            ),
          ],
        ),
      ),
    );
  }
}

String _shortDate(DateTime value) {
  final local = value.toLocal();
  final day = local.day.toString().padLeft(2, '0');
  final month = local.month.toString().padLeft(2, '0');
  final hour = local.hour.toString().padLeft(2, '0');
  final minute = local.minute.toString().padLeft(2, '0');
  return '$day.$month $hour:$minute';
}
