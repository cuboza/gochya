import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../app/theme.dart';
import '../../core/models/onboarding_models.dart';
import '../../core/models/profile_models.dart';
import '../../core/network/gochya_api_client.dart';
import '../care/care_actions.dart';
import '../onboarding/onboarding_repository.dart';
import '../onboarding/onboarding_screen.dart';
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
            accessToken: accessToken,
            snapshot: value,
            onCareChanged: () {
              ref.invalidate(homeSnapshotProvider(accessToken));
            },
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
  const _HomeContent({
    required this.accessToken,
    required this.snapshot,
    required this.onCareChanged,
    required this.onOpenLineage,
  });

  final String accessToken;
  final HomeSnapshot snapshot;
  final VoidCallback onCareChanged;
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
          _NoPetState(accessToken: accessToken)
        else ...[
          _PetHero(pet: pet),
          const SizedBox(height: 16),
          _NeedsCard(needs: pet.needs),
          const SizedBox(height: 16),
          CareActions(
            accountId: snapshot.profile.id,
            accessToken: accessToken,
            pet: pet,
            onSnapshotChanged: onCareChanged,
          ),
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

class _NoPetState extends ConsumerWidget {
  const _NoPetState({required this.accessToken});

  final String accessToken;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final eggs = ref.watch(onboardingEggsProvider(accessToken));
    return eggs.when(
      loading: () => const Card(
        child: Padding(
          padding: EdgeInsets.all(24),
          child: Column(
            children: [
              CircularProgressIndicator(),
              SizedBox(height: 16),
              Text('Проверяем инкубатор…'),
            ],
          ),
        ),
      ),
      error: (error, stackTrace) => Card(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            children: [
              const Icon(Icons.cloud_off_outlined, size: 52),
              const SizedBox(height: 12),
              const Text(
                'Не удалось проверить инкубатор',
                style: TextStyle(fontWeight: FontWeight.w800),
              ),
              const SizedBox(height: 12),
              FilledButton.tonalIcon(
                onPressed: () {
                  ref.invalidate(onboardingEggsProvider(accessToken));
                },
                icon: const Icon(Icons.refresh_rounded),
                label: const Text('Повторить'),
              ),
            ],
          ),
        ),
      ),
      data: (values) {
        if (values.isNotEmpty) {
          return _IncubatingEggCard(
            accessToken: accessToken,
            egg: values.first,
          );
        }
        return _NoPetCard(
          onStart: () async {
            final result = await Navigator.of(context).push<StarterEggResult>(
              MaterialPageRoute<StarterEggResult>(
                builder: (context) =>
                    OnboardingScreen(accessToken: accessToken),
              ),
            );
            if (result != null) {
              ref.invalidate(onboardingEggsProvider(accessToken));
            }
          },
        );
      },
    );
  }
}

class _NoPetCard extends StatelessWidget {
  const _NoPetCard({required this.onStart});

  final VoidCallback onStart;

  @override
  Widget build(BuildContext context) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          children: [
            const Icon(Icons.egg_outlined, size: 56),
            const SizedBox(height: 12),
            const Text(
              'У тебя пока нет питомца',
              style: TextStyle(fontSize: 18, fontWeight: FontWeight.w800),
            ),
            const SizedBox(height: 4),
            const Text(
              'Стартовый питомец появится после завершения онбординга.',
              textAlign: TextAlign.center,
              style: TextStyle(color: GochyaColors.muted),
            ),
            const SizedBox(height: 18),
            FilledButton.icon(
              onPressed: onStart,
              icon: const Icon(Icons.auto_awesome_rounded),
              label: const Text('Создать первого питомца'),
            ),
          ],
        ),
      ),
    );
  }
}

class _IncubatingEggCard extends ConsumerStatefulWidget {
  const _IncubatingEggCard({required this.accessToken, required this.egg});

  final String accessToken;
  final EggSummary egg;

  @override
  ConsumerState<_IncubatingEggCard> createState() => _IncubatingEggCardState();
}

class _IncubatingEggCardState extends ConsumerState<_IncubatingEggCard> {
  Timer? _timer;
  late DateTime _now;
  var _isHatching = false;
  Object? _error;

  @override
  void initState() {
    super.initState();
    _now = DateTime.now();
    _scheduleTick();
  }

  @override
  void didUpdateWidget(covariant _IncubatingEggCard oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.egg.id != widget.egg.id ||
        oldWidget.egg.incubateUntil != widget.egg.incubateUntil) {
      _now = DateTime.now();
      _scheduleTick();
    }
  }

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final remaining = widget.egg.incubateUntil.difference(_now);
    final isReady = remaining <= Duration.zero;
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          children: [
            Icon(
              isReady ? Icons.egg_rounded : Icons.egg_outlined,
              size: 64,
              color: isReady ? GochyaColors.secondary : null,
            ),
            const SizedBox(height: 12),
            Text(
              isReady ? 'Яйцо готово!' : 'Яйцо в инкубаторе',
              style: const TextStyle(fontSize: 20, fontWeight: FontWeight.w800),
            ),
            const SizedBox(height: 6),
            Text(
              isReady
                  ? 'Пора познакомиться с первым питомцем.'
                  : 'До вылупления: ${_remainingLabel(remaining)}',
              textAlign: TextAlign.center,
              style: const TextStyle(color: GochyaColors.muted),
            ),
            if (_error != null) ...[
              const SizedBox(height: 12),
              Text(
                _hatchErrorMessage(_error!),
                textAlign: TextAlign.center,
                style: const TextStyle(color: GochyaColors.warning),
              ),
            ],
            const SizedBox(height: 18),
            FilledButton.icon(
              onPressed: !isReady || _isHatching ? null : _hatch,
              icon: _isHatching
                  ? const SizedBox.square(
                      dimension: 18,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    )
                  : const Icon(Icons.celebration_rounded),
              label: Text(_isHatching ? 'Вылупляем…' : 'Вылупить питомца'),
            ),
          ],
        ),
      ),
    );
  }

  void _scheduleTick() {
    _timer?.cancel();
    if (widget.egg.isReadyAt(_now)) {
      return;
    }
    _timer = Timer.periodic(const Duration(seconds: 1), (timer) {
      if (!mounted) {
        return;
      }
      setState(() {
        _now = DateTime.now();
      });
      if (widget.egg.isReadyAt(_now)) {
        timer.cancel();
      }
    });
  }

  Future<void> _hatch() async {
    setState(() {
      _isHatching = true;
      _error = null;
    });
    try {
      await ref
          .read(onboardingRepositoryProvider)
          .hatchEgg(widget.accessToken, widget.egg.id);
      if (!mounted) {
        return;
      }
      ref.invalidate(onboardingEggsProvider(widget.accessToken));
      ref.invalidate(homeSnapshotProvider(widget.accessToken));
    } catch (error) {
      if (!mounted) {
        return;
      }
      setState(() {
        _isHatching = false;
        _error = error;
        _now = DateTime.now();
      });
    }
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

String _remainingLabel(Duration value) {
  final seconds = value.inSeconds + (value.inMilliseconds % 1000 == 0 ? 0 : 1);
  if (seconds < 60) {
    return '$seconds сек.';
  }
  final minutes = (seconds / 60).ceil();
  return '$minutes мин.';
}

String _hatchErrorMessage(Object error) {
  if (error is ApiException && error.code == 'egg_not_ready') {
    return 'Сервер ещё завершает инкубацию. Попробуй через секунду.';
  }
  if (error is ApiException && error.isUnauthorized) {
    return 'Сессия истекла. Войди заново, чтобы продолжить.';
  }
  return 'Не удалось вылупить питомца. Проверь соединение и повтори.';
}
