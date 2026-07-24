import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../app/theme.dart';
import '../../core/models/profile_models.dart';
import '../../core/network/gochya_api_client.dart';
import '../session/session_controller.dart';
import 'lineage_screen.dart';
import 'profile_repository.dart';

class HomeScreen extends ConsumerWidget {
  const HomeScreen({required this.accessToken, super.key});

  final String accessToken;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final snapshot = ref.watch(homeSnapshotProvider(accessToken));
    return Scaffold(
      appBar: AppBar(
        title: const Text(
          'GOCHYA',
          style: TextStyle(fontWeight: FontWeight.w900, letterSpacing: 1.4),
        ),
      ),
      body: snapshot.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (error, stackTrace) => _HomeError(
          error: error,
          onRetry: () => ref.invalidate(homeSnapshotProvider(accessToken)),
          onSignOut: () {
            ref.read(sessionControllerProvider.notifier).signOut();
          },
        ),
        data: (value) => RefreshIndicator(
          onRefresh: () =>
              ref.refresh(homeSnapshotProvider(accessToken).future),
          child: _HomeContent(
            snapshot: value,
            onOpenLineage: (pet) {
              Navigator.of(context).push(
                MaterialPageRoute<void>(
                  builder: (context) =>
                      LineageScreen(accessToken: accessToken, pet: pet),
                ),
              );
            },
          ),
        ),
      ),
    );
  }
}

class _HomeContent extends StatelessWidget {
  const _HomeContent({required this.snapshot, required this.onOpenLineage});

  final HomeSnapshot snapshot;
  final ValueChanged<PetSummary> onOpenLineage;

  @override
  Widget build(BuildContext context) {
    final pet = snapshot.activePet;
    return ListView(
      physics: const AlwaysScrollableScrollPhysics(),
      padding: const EdgeInsets.fromLTRB(20, 8, 20, 28),
      children: [
        Text(
          'Привет, ${snapshot.profile.label}',
          style: Theme.of(
            context,
          ).textTheme.headlineSmall?.copyWith(fontWeight: FontWeight.w800),
        ),
        const SizedBox(height: 4),
        Text(
          snapshot.profile.streakDays == 0
              ? 'Начни серию заботы сегодня'
              : 'Серия заботы: ${snapshot.profile.streakDays} дн.',
          style: Theme.of(
            context,
          ).textTheme.bodyLarge?.copyWith(color: GochyaColors.secondary),
        ),
        const SizedBox(height: 20),
        if (pet == null)
          const _NoPetCard()
        else ...[
          _PetHero(pet: pet),
          const SizedBox(height: 16),
          _NeedsCard(needs: pet.needs),
          const SizedBox(height: 16),
          Card(
            child: Padding(
              padding: const EdgeInsets.all(20),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    'История рода',
                    style: Theme.of(context).textTheme.titleLarge?.copyWith(
                      fontWeight: FontWeight.w800,
                    ),
                  ),
                  const SizedBox(height: 8),
                  const Text(
                    'До трёх поколений, без приватного состояния предков.',
                  ),
                  const SizedBox(height: 16),
                  FilledButton.tonalIcon(
                    onPressed: () => onOpenLineage(pet),
                    icon: const Icon(Icons.account_tree_outlined),
                    label: const Text('Открыть родословную'),
                  ),
                ],
              ),
            ),
          ),
        ],
      ],
    );
  }
}

class _PetHero extends StatelessWidget {
  const _PetHero({required this.pet});

  final PetSummary pet;

  @override
  Widget build(BuildContext context) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Row(
          children: [
            Container(
              width: 88,
              height: 88,
              decoration: BoxDecoration(
                gradient: const LinearGradient(
                  begin: Alignment.topLeft,
                  end: Alignment.bottomRight,
                  colors: [GochyaColors.primary, Color(0xFF5B8DEF)],
                ),
                borderRadius: BorderRadius.circular(28),
              ),
              child: const Icon(Icons.pets_rounded, size: 48),
            ),
            const SizedBox(width: 18),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    pet.label,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: Theme.of(context).textTheme.headlineSmall?.copyWith(
                      fontWeight: FontWeight.w900,
                    ),
                  ),
                  const SizedBox(height: 4),
                  Text(
                    '${_stageLabel(pet.stage)} · уровень ${pet.level}',
                    style: const TextStyle(color: GochyaColors.muted),
                  ),
                  const SizedBox(height: 4),
                  Text(
                    'Поколение ${pet.generation}',
                    style: const TextStyle(color: GochyaColors.muted),
                  ),
                  if (pet.isWeak) ...[
                    const SizedBox(height: 8),
                    const Row(
                      children: [
                        Icon(
                          Icons.warning_amber_rounded,
                          size: 18,
                          color: GochyaColors.warning,
                        ),
                        SizedBox(width: 6),
                        Text(
                          'Нужна забота',
                          style: TextStyle(color: GochyaColors.warning),
                        ),
                      ],
                    ),
                  ],
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _NeedsCard extends StatelessWidget {
  const _NeedsCard({required this.needs});

  final PetNeeds needs;

  @override
  Widget build(BuildContext context) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              'Состояние',
              style: Theme.of(
                context,
              ).textTheme.titleLarge?.copyWith(fontWeight: FontWeight.w800),
            ),
            const SizedBox(height: 18),
            _NeedIndicator(
              label: 'Сытость',
              value: needs.hunger,
              color: GochyaColors.hunger,
            ),
            _NeedIndicator(
              label: 'Энергия',
              value: needs.energy,
              color: GochyaColors.energy,
            ),
            _NeedIndicator(
              label: 'Гигиена',
              value: needs.hygiene,
              color: GochyaColors.hygiene,
            ),
            _NeedIndicator(
              label: 'Настроение',
              value: needs.mood,
              color: GochyaColors.mood,
              bottomPadding: 0,
            ),
          ],
        ),
      ),
    );
  }
}

class _NeedIndicator extends StatelessWidget {
  const _NeedIndicator({
    required this.label,
    required this.value,
    required this.color,
    this.bottomPadding = 14,
  });

  final String label;
  final int value;
  final Color color;
  final double bottomPadding;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: EdgeInsets.only(bottom: bottomPadding),
      child: Semantics(
        label: '$label: $value из 100',
        child: Column(
          children: [
            Row(
              children: [
                Expanded(child: Text(label)),
                Text(
                  '$value%',
                  style: const TextStyle(fontWeight: FontWeight.w700),
                ),
              ],
            ),
            const SizedBox(height: 7),
            LinearProgressIndicator(
              value: value / 100,
              minHeight: 9,
              borderRadius: BorderRadius.circular(10),
              color: color,
              backgroundColor: color.withValues(alpha: 0.14),
            ),
          ],
        ),
      ),
    );
  }
}

class _NoPetCard extends StatelessWidget {
  const _NoPetCard();

  @override
  Widget build(BuildContext context) {
    return const Card(
      child: Padding(
        padding: EdgeInsets.all(24),
        child: Column(
          children: [
            Icon(Icons.egg_outlined, size: 56),
            SizedBox(height: 12),
            Text('У тебя пока нет питомца'),
            SizedBox(height: 4),
            Text(
              'Стартовый питомец появится после завершения онбординга.',
              textAlign: TextAlign.center,
              style: TextStyle(color: GochyaColors.muted),
            ),
          ],
        ),
      ),
    );
  }
}

class _HomeError extends StatelessWidget {
  const _HomeError({
    required this.error,
    required this.onRetry,
    required this.onSignOut,
  });

  final Object error;
  final VoidCallback onRetry;
  final VoidCallback onSignOut;

  @override
  Widget build(BuildContext context) {
    final apiError = error is ApiException ? error as ApiException : null;
    final unauthorized = apiError?.isUnauthorized ?? false;
    final requestId = apiError?.requestId;
    return Center(
      child: SingleChildScrollView(
        padding: const EdgeInsets.all(28),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              unauthorized
                  ? Icons.lock_clock_outlined
                  : Icons.cloud_off_outlined,
              size: 64,
            ),
            const SizedBox(height: 16),
            Text(
              unauthorized ? 'Сессия истекла' : 'Не удалось загрузить данные',
              textAlign: TextAlign.center,
              style: Theme.of(
                context,
              ).textTheme.titleLarge?.copyWith(fontWeight: FontWeight.w800),
            ),
            const SizedBox(height: 8),
            Text(
              unauthorized
                  ? 'Войди заново, чтобы продолжить.'
                  : 'Проверь соединение и повтори запрос.',
              textAlign: TextAlign.center,
              style: const TextStyle(color: GochyaColors.muted),
            ),
            if (requestId != null) ...[
              const SizedBox(height: 8),
              Text(
                'Request ID: $requestId',
                style: Theme.of(context).textTheme.bodySmall,
              ),
            ],
            const SizedBox(height: 20),
            FilledButton.icon(
              onPressed: unauthorized ? onSignOut : onRetry,
              icon: Icon(
                unauthorized ? Icons.logout_rounded : Icons.refresh_rounded,
              ),
              label: Text(unauthorized ? 'Очистить сессию' : 'Повторить'),
            ),
          ],
        ),
      ),
    );
  }
}

String _stageLabel(String stage) {
  return switch (stage.toLowerCase()) {
    'egg' => 'Яйцо',
    'baby' => 'Малыш',
    'teen' => 'Подросток',
    'adult' => 'Взрослый',
    _ => stage,
  };
}
