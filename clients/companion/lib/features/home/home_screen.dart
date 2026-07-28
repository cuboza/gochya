import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../app/gochya_loader.dart';
import '../../app/theme.dart';
import '../../core/ffi/core_provider.dart';
import '../../core/ffi/gochya_core.dart';
import '../../core/models/care_models.dart';
import '../../core/models/onboarding_models.dart';
import '../../core/models/profile_models.dart';
import '../../core/network/gochya_api_client.dart';
import '../activity/activity_screen.dart';
import '../care/care_actions.dart';
import '../creatures/creature_art.dart';
import '../creatures/creature_rig.dart';
import '../creatures/rigged_creature.dart';
import '../onboarding/onboarding_repository.dart';
import '../onboarding/onboarding_screen.dart';
import '../session/session_controller.dart';
import 'need_gauge.dart';
import 'needs_prediction.dart';
import 'profile_repository.dart';
import 'symbiosis_card.dart';

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
        actions: [
          // The care streak is the one number worth carrying everywhere, so it
          // rides in the bar instead of a greeting block that repeated the
          // player's own name to them on every visit.
          if (snapshot.value case final loaded?)
            _StreakChip(days: loaded.profile.streakDays),
          // Symbiosis is a headline mechanic, so it gets a permanent entry
          // point instead of living only in a card below the fold.
          IconButton(
            onPressed: () {
              Navigator.of(context).push(
                MaterialPageRoute<void>(
                  builder: (context) =>
                      ActivityScreen(accessToken: accessToken),
                ),
              );
            },
            icon: const Icon(Icons.monitor_heart_outlined),
            tooltip: 'Активность и Vitality',
          ),
        ],
      ),
      body: snapshot.when(
        // A care action reloads the profile. Dropping back to a spinner would
        // blank the pet mid-reaction, so previously loaded data stays on
        // screen while the reload runs.
        skipLoadingOnReload: true,
        loading: () => const GochyaLoader(caption: 'Будим питомца…'),
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
            core: ref.watch(gochyaCoreProvider),
            onCareChanged: () {
              ref.invalidate(homeSnapshotProvider(accessToken));
            },
          ),
        ),
      ),
    );
  }
}

class _HomeContent extends StatefulWidget {
  const _HomeContent({
    required this.accessToken,
    required this.snapshot,
    required this.core,
    required this.onCareChanged,
  });

  final String accessToken;
  final HomeSnapshot snapshot;
  final GochyaCore? core;
  final VoidCallback onCareChanged;

  @override
  State<_HomeContent> createState() => _HomeContentState();
}

class _HomeContentState extends State<_HomeContent>
    with SingleTickerProviderStateMixin {
  /// Reaction lengths come from `ART_BIBLE.md` §9.2.
  static const _durations = <CareOperation, Duration>{
    CareOperation.feed: Duration(milliseconds: 600),
    CareOperation.clean: Duration(milliseconds: 900),
    CareOperation.play: Duration(milliseconds: 800),
  };

  // Built in initState, not lazily: a lazy field would be constructed inside
  // dispose() on a screen that never showed a pet.
  late final AnimationController _reaction;
  CreatureAction? _action;

  @override
  void initState() {
    super.initState();
    _reaction = AnimationController(vsync: this)
      ..addStatusListener((status) {
        if (status == AnimationStatus.completed && mounted) {
          setState(() => _action = null);
        }
      });
  }

  @override
  void dispose() {
    _reaction.dispose();
    super.dispose();
  }

  void _playReaction(CareOperation operation) {
    // Reduce motion skips the reaction outright (`ART_BIBLE.md` §9.3). Relying
    // on Flutter shortening the controller to 5% is not enough: the reaction
    // would still run, and its particles would flash across the screen.
    // Nothing is lost by skipping — the care result arrives from the server
    // either way, and the needs card shows it.
    if (MediaQuery.disableAnimationsOf(context)) {
      return;
    }
    final duration = _durations[operation];
    final action = switch (operation) {
      CareOperation.feed => CreatureAction.eat,
      CareOperation.clean => CreatureAction.clean,
      CareOperation.play => CreatureAction.play,
      // Sleep is a lasting state, read from the pet, not a one-shot reaction.
      CareOperation.sleep => null,
    };
    if (action == null || duration == null) {
      return;
    }
    setState(() => _action = action);
    _reaction
      ..duration = duration
      ..forward(from: 0);
  }

  @override
  Widget build(BuildContext context) {
    final snapshot = widget.snapshot;
    final accessToken = widget.accessToken;
    final pet = snapshot.activePet;
    return ListView(
      physics: const AlwaysScrollableScrollPhysics(),
      padding: const EdgeInsets.fromLTRB(20, 8, 20, 28),
      children: [
        if (pet == null)
          _NoPetState(accessToken: accessToken)
        else ...[
          // Needs live inside the pet card rather than in one of their own.
          // They describe the creature above them, and a separate titled card
          // pushed the daily care actions a full screen further down.
          // Decay between profile reads is predicted by the Core, so the pet
          // does not sit frozen at whatever the last response said.
          _PetHero(
            pet: pet,
            reaction: _action,
            reactionProgress: _reaction,
            needs: predictNeeds(
              core: widget.core,
              pet: pet,
              now: DateTime.now().toUtc(),
            ),
          ),
          const SizedBox(height: 16),
          CareActions(
            accountId: snapshot.profile.id,
            accessToken: accessToken,
            pet: pet,
            onSnapshotChanged: widget.onCareChanged,
            onCareApplied: _playReaction,
          ),
          const SizedBox(height: 16),
          SymbiosisCard(accessToken: accessToken),
          // Lineage left the main screen: it is something a player looks up
          // once in a while, not a daily action, and it now hangs off the pet
          // in the profile where every pet is already listed.
        ],
      ],
    );
  }
}

/// The care streak, small enough to live in the app bar.
class _StreakChip extends StatelessWidget {
  const _StreakChip({required this.days});

  final int days;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      label: days == 0 ? 'Серии заботы пока нет' : 'Серия заботы: $days дней',
      child: ExcludeSemantics(
        child: Padding(
          padding: const EdgeInsets.symmetric(vertical: 8),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(
                Icons.local_fire_department_rounded,
                size: 20,
                color: GochyaColors.secondary,
              ),
              const SizedBox(width: 4),
              Text(
                '$days',
                style: Theme.of(context).textTheme.titleMedium?.copyWith(
                  color: GochyaColors.secondary,
                  fontWeight: FontWeight.w800,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _PetHero extends StatelessWidget {
  const _PetHero({
    required this.pet,
    required this.reaction,
    required this.reactionProgress,
    required this.needs,
  });

  final PetSummary pet;
  final PetNeeds needs;

  /// One-shot care reaction, or `null` when the pet is just idling.
  final CreatureAction? reaction;
  final Animation<double> reactionProgress;

  @override
  Widget build(BuildContext context) {
    final element = creatureElementOf(pet.genome);
    final tint = element?.tint ?? GochyaColors.primary;
    final sleepingUntil = pet.sleepingUntil;
    final isSleeping =
        sleepingUntil != null && sleepingUntil.isAfter(DateTime.now());
    return Card(
      clipBehavior: Clip.antiAlias,
      child: Column(
        children: [
          // The pet is the first thing the player meets, so it gets a full
          // stage instead of an avatar-sized thumbnail.
          Container(
            height: 240,
            width: double.infinity,
            decoration: BoxDecoration(
              gradient: LinearGradient(
                begin: Alignment.topCenter,
                end: Alignment.bottomCenter,
                colors: [
                  tint.withValues(alpha: 0.32),
                  GochyaColors.backgroundMid,
                ],
              ),
            ),
            child: Stack(
              alignment: Alignment.bottomCenter,
              children: [
                Padding(
                  padding: const EdgeInsets.fromLTRB(24, 18, 24, 12),
                  child: Semantics(
                    label: element == null
                        ? pet.label
                        : '${pet.label}, ${element.label}',
                    child: Center(
                      child: AnimatedBuilder(
                        animation: reactionProgress,
                        builder: (context, _) => RiggedCreature(
                          element: element,
                          width: 260,
                          // A sleeping pet keeps sleeping: a lasting state
                          // outranks a one-shot reaction.
                          action: isSleeping
                              ? CreatureAction.sleeping
                              : reaction ?? CreatureAction.idle,
                          actionProgress: reactionProgress.value,
                        ),
                      ),
                    ),
                  ),
                ),
                if (reaction == CreatureAction.eat)
                  _FlyingTreat(progress: reactionProgress),
                if (element != null)
                  Positioned(
                    top: 12,
                    right: 14,
                    child: Container(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 12,
                        vertical: 5,
                      ),
                      decoration: BoxDecoration(
                        color: GochyaColors.background.withValues(alpha: 0.62),
                        borderRadius: BorderRadius.circular(20),
                      ),
                      child: Text(
                        element.label,
                        style: TextStyle(
                          color: tint,
                          fontWeight: FontWeight.w800,
                          fontSize: 12,
                        ),
                      ),
                    ),
                  ),
              ],
            ),
          ),
          Padding(
            padding: const EdgeInsets.all(20),
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
                  '${pet.stageLabel} · уровень ${pet.level}',
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
                const SizedBox(height: 18),
                // Four rings in a row, as in the V1 concept layout the art
                // bible still calls the reference. Wrap, not Row: at a large
                // font scale they fall into two rows instead of overflowing.
                Wrap(
                  alignment: WrapAlignment.spaceBetween,
                  spacing: 12,
                  runSpacing: 14,
                  children: [
                    NeedGauge(
                      label: 'Сытость',
                      icon: Icons.restaurant_rounded,
                      value: needs.hunger,
                      color: GochyaColors.hunger,
                    ),
                    NeedGauge(
                      label: 'Энергия',
                      icon: Icons.bedtime_rounded,
                      value: needs.energy,
                      color: GochyaColors.energy,
                    ),
                    NeedGauge(
                      label: 'Гигиена',
                      icon: Icons.water_drop_rounded,
                      value: needs.hygiene,
                      color: GochyaColors.hygiene,
                    ),
                    NeedGauge(
                      label: 'Настроение',
                      icon: Icons.favorite_rounded,
                      value: needs.mood,
                      color: GochyaColors.mood,
                    ),
                  ],
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

/// The apple that flies into the pet's mouth while it eats
/// (`ART_BIBLE.md` §9.2).
class _FlyingTreat extends StatelessWidget {
  const _FlyingTreat({required this.progress});

  final Animation<double> progress;

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: progress,
      builder: (context, _) {
        final t = Curves.easeInCubic.transform(
          (progress.value / 0.55).clamp(0.0, 1.0),
        );
        final fade = (1 - ((progress.value - 0.5) / 0.3)).clamp(0.0, 1.0);
        return Align(
          alignment: Alignment.lerp(
            const Alignment(-0.85, 0.7),
            const Alignment(0.1, -0.05),
            t,
          )!,
          child: Opacity(
            opacity: fade,
            child: Transform.rotate(
              angle: t * 2.4,
              child: const Icon(
                Icons.apple_rounded,
                size: 30,
                color: GochyaColors.hunger,
              ),
            ),
          ),
        );
      },
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
            children: [GochyaLoader(caption: 'Проверяем инкубатор…')],
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
