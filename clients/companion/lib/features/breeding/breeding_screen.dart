import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../app/gochya_loader.dart';
import '../../app/theme.dart';
import '../../core/identifiers/uuid_v4.dart';
import '../../core/models/breeding_models.dart';
import '../../core/models/onboarding_models.dart';
import '../../core/models/profile_models.dart';
import '../../core/network/gochya_api_client.dart';
import '../home/profile_repository.dart';
import 'breeding_repository.dart';

/// Breeding is fully server-authoritative: the phone picks two parents and
/// optional catalysts, the server spends the ledgers and derives the genome.
class BreedingScreen extends ConsumerStatefulWidget {
  const BreedingScreen({required this.accessToken, super.key});

  final String accessToken;

  @override
  ConsumerState<BreedingScreen> createState() => _BreedingScreenState();
}

class _BreedingScreenState extends ConsumerState<BreedingScreen> {
  String? _parentAId;
  String? _parentBId;
  final _catalysts = <BreedingCatalyst>{};
  String? _pendingIdempotencyKey;
  String? _breedError;
  BreedingResult? _lastResult;
  var _isBreeding = false;

  @override
  Widget build(BuildContext context) {
    final snapshot = ref.watch(breedingSnapshotProvider(widget.accessToken));
    return Scaffold(
      appBar: AppBar(title: const Text('Бридинг')),
      body: snapshot.when(
        loading: () => const GochyaLoader(caption: 'Считаем родословные…'),
        error: (error, stackTrace) => _BreedingError(
          message: _loadMessage(error),
          onRetry: () =>
              ref.invalidate(breedingSnapshotProvider(widget.accessToken)),
        ),
        data: (value) => RefreshIndicator(
          onRefresh: () =>
              ref.refresh(breedingSnapshotProvider(widget.accessToken).future),
          child: ListView(
            physics: const AlwaysScrollableScrollPhysics(),
            padding: const EdgeInsets.fromLTRB(20, 12, 20, 32),
            children: [
              _CostCard(snapshot: value),
              const SizedBox(height: 16),
              _ParentPicker(
                title: 'Родитель A',
                pets: value.eligibleParents,
                selectedId: _parentAId,
                excludedId: _parentBId,
                onSelected: (id) => setState(() {
                  _parentAId = id;
                  _breedError = null;
                }),
              ),
              const SizedBox(height: 12),
              _ParentPicker(
                title: 'Родитель B',
                pets: value.eligibleParents,
                selectedId: _parentBId,
                excludedId: _parentAId,
                onSelected: (id) => setState(() {
                  _parentBId = id;
                  _breedError = null;
                }),
              ),
              const SizedBox(height: 16),
              _CatalystCard(
                selected: _catalysts,
                onToggle: (catalyst, enabled) => setState(() {
                  if (enabled) {
                    _catalysts.add(catalyst);
                  } else {
                    _catalysts.remove(catalyst);
                  }
                  _breedError = null;
                }),
              ),
              if (_breedError != null) ...[
                const SizedBox(height: 12),
                Text(
                  _breedError!,
                  textAlign: TextAlign.center,
                  style: TextStyle(color: Theme.of(context).colorScheme.error),
                ),
              ],
              const SizedBox(height: 16),
              FilledButton.icon(
                onPressed: _canSubmit(value) ? _breed : null,
                icon: _isBreeding
                    ? const SizedBox.square(
                        dimension: 18,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      )
                    : const Icon(Icons.favorite_rounded),
                label: Text(
                  _pendingIdempotencyKey != null && !_isBreeding
                      ? 'Повторить скрещивание'
                      : 'Скрестить за $breedCostKoins Koins',
                ),
              ),
              if (_lastResult != null) ...[
                const SizedBox(height: 16),
                _NewEggBanner(result: _lastResult!),
              ],
              const SizedBox(height: 24),
              Text(
                'Инкубатор · ${value.eggs.length}',
                style: Theme.of(
                  context,
                ).textTheme.titleLarge?.copyWith(fontWeight: FontWeight.w800),
              ),
              const SizedBox(height: 12),
              if (value.eggs.isEmpty)
                const _EmptyIncubatorCard()
              else
                for (final egg in value.eggs) ...[
                  _EggCard(
                    accessToken: widget.accessToken,
                    egg: egg,
                    onHatched: () {
                      ref.invalidate(
                        breedingSnapshotProvider(widget.accessToken),
                      );
                      ref.invalidate(homeSnapshotProvider(widget.accessToken));
                    },
                  ),
                  const SizedBox(height: 10),
                ],
            ],
          ),
        ),
      ),
    );
  }

  bool _canSubmit(BreedingSnapshot snapshot) {
    return !_isBreeding &&
        _parentAId != null &&
        _parentBId != null &&
        _parentAId != _parentBId &&
        snapshot.canAffordBreeding;
  }

  Future<void> _breed() async {
    final parentAId = _parentAId;
    final parentBId = _parentBId;
    if (parentAId == null || parentBId == null) {
      return;
    }
    // The key survives a failed attempt so a lost response cannot spend the
    // Koins and the Love Crystal twice.
    final idempotencyKey = _pendingIdempotencyKey ?? newUuidV4();
    setState(() {
      _isBreeding = true;
      _pendingIdempotencyKey = idempotencyKey;
      _breedError = null;
    });
    try {
      final result = await ref
          .read(breedingRepositoryProvider)
          .breed(
            accessToken: widget.accessToken,
            parentAId: parentAId,
            parentBId: parentBId,
            catalysts: _catalysts.toList(growable: false),
            idempotencyKey: idempotencyKey,
          );
      if (!mounted) {
        return;
      }
      setState(() {
        _isBreeding = false;
        _pendingIdempotencyKey = null;
        _lastResult = result;
      });
      ref.invalidate(breedingSnapshotProvider(widget.accessToken));
    } on Object catch (error) {
      if (!mounted) {
        return;
      }
      setState(() {
        _isBreeding = false;
        _breedError = _breedMessage(error);
      });
    }
  }

  String _breedMessage(Object error) {
    if (error is! ApiException) {
      return 'Исход неизвестен. Повтори — тот же ключ не спишет ресурсы дважды.';
    }
    return switch (error.code) {
      'insufficient_koins' => 'Не хватает Koins: нужно $breedCostKoins.',
      'love_crystal_required' => 'Нужен Кристалл любви — купи его в магазине.',
      'catalyst_required' => 'Выбранного катализатора нет в инвентаре.',
      'parent_ineligible' =>
        'Оба родителя должны быть здоровыми взрослыми уровня '
            '$breedMinimumParentLevel.',
      'parent_cooldown' => 'Родитель ещё на кулдауне (24 часа).',
      'parents_too_related' => 'Эти питомцы слишком близкие родственники.',
      'parents_identical' => 'Нужны два разных питомца.',
      'parent_not_found' => 'Питомец не найден. Обнови экран.',
      'core_unavailable' => 'Ядро недоступно. Повтори позже.',
      'request_timeout' || 'network_error' =>
        'Исход неизвестен. Повтори — тот же ключ не спишет ресурсы дважды.',
      _ => 'Не удалось создать яйцо.',
    };
  }

  String _loadMessage(Object error) {
    if (error is ApiException &&
        (error.code == 'request_timeout' || error.code == 'network_error')) {
      return 'Бридинг требует связи с сервером.';
    }
    return 'Не удалось загрузить питомцев и инкубатор.';
  }
}

class _CostCard extends StatelessWidget {
  const _CostCard({required this.snapshot});

  final BreedingSnapshot snapshot;

  @override
  Widget build(BuildContext context) {
    final affordable = snapshot.canAffordBreeding;
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(
                  affordable
                      ? Icons.check_circle_rounded
                      : Icons.error_outline_rounded,
                  color: affordable
                      ? GochyaColors.success
                      : GochyaColors.warning,
                ),
                const SizedBox(width: 10),
                Expanded(
                  child: Text(
                    'Стоимость: $breedCostKoins Koins + Кристалл любви',
                    style: Theme.of(context).textTheme.titleMedium?.copyWith(
                      fontWeight: FontWeight.w800,
                    ),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 10),
            Text(
              'Баланс: ${snapshot.inventory.koins} Koins · '
              'кристаллов ${snapshot.loveCrystals}',
              style: const TextStyle(color: GochyaColors.muted),
            ),
            const SizedBox(height: 6),
            Text(
              'Подходящих родителей: ${snapshot.eligibleParents.length} '
              '(здоровые взрослые от уровня $breedMinimumParentLevel).',
              style: const TextStyle(color: GochyaColors.muted),
            ),
          ],
        ),
      ),
    );
  }
}

class _ParentPicker extends StatelessWidget {
  const _ParentPicker({
    required this.title,
    required this.pets,
    required this.selectedId,
    required this.excludedId,
    required this.onSelected,
  });

  final String title;
  final List<PetSummary> pets;
  final String? selectedId;
  final String? excludedId;
  final ValueChanged<String> onSelected;

  @override
  Widget build(BuildContext context) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              title,
              style: Theme.of(
                context,
              ).textTheme.titleMedium?.copyWith(fontWeight: FontWeight.w800),
            ),
            const SizedBox(height: 12),
            if (pets.isEmpty)
              const Text(
                'Нет подходящих питомцев.',
                style: TextStyle(color: GochyaColors.muted),
              )
            else
              Wrap(
                spacing: 8,
                runSpacing: 8,
                children: [
                  for (final pet in pets)
                    ChoiceChip(
                      selected: pet.id == selectedId,
                      onSelected: pet.id == excludedId
                          ? null
                          : (_) => onSelected(pet.id),
                      label: Text('${pet.label} · ур. ${pet.level}'),
                    ),
                ],
              ),
          ],
        ),
      ),
    );
  }
}

class _CatalystCard extends StatelessWidget {
  const _CatalystCard({required this.selected, required this.onToggle});

  final Set<BreedingCatalyst> selected;
  final void Function(BreedingCatalyst catalyst, bool enabled) onToggle;

  @override
  Widget build(BuildContext context) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.fromLTRB(20, 16, 20, 8),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              'Катализаторы',
              style: Theme.of(
                context,
              ).textTheme.titleMedium?.copyWith(fontWeight: FontWeight.w800),
            ),
            const SizedBox(height: 4),
            const Text(
              'Расходуются из инвентаря. Сервер откажет, если предмета нет.',
              style: TextStyle(color: GochyaColors.muted, fontSize: 12),
            ),
            for (final catalyst in BreedingCatalyst.values)
              SwitchListTile(
                contentPadding: EdgeInsets.zero,
                value: selected.contains(catalyst),
                onChanged: (value) => onToggle(catalyst, value),
                title: Text(catalyst.label),
              ),
          ],
        ),
      ),
    );
  }
}

class _NewEggBanner extends StatelessWidget {
  const _NewEggBanner({required this.result});

  final BreedingResult result;

  @override
  Widget build(BuildContext context) {
    return Card(
      color: GochyaColors.primary.withValues(alpha: 0.2),
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Row(
          children: [
            const Icon(Icons.egg_rounded, color: GochyaColors.secondary),
            const SizedBox(width: 12),
            Expanded(
              child: Text(
                'Яйцо создано. Инкубация до '
                '${_shortDate(result.incubateUntil)}.',
                style: const TextStyle(fontWeight: FontWeight.w700),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _EggCard extends ConsumerStatefulWidget {
  const _EggCard({
    required this.accessToken,
    required this.egg,
    required this.onHatched,
  });

  final String accessToken;
  final EggSummary egg;
  final VoidCallback onHatched;

  @override
  ConsumerState<_EggCard> createState() => _EggCardState();
}

class _EggCardState extends ConsumerState<_EggCard> {
  Timer? _timer;
  late DateTime _now;
  var _isHatching = false;
  String? _error;

  @override
  void initState() {
    super.initState();
    _now = DateTime.now();
    _scheduleTick();
  }

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final isReady = widget.egg.isReadyAt(_now);
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Row(
          children: [
            Icon(
              isReady ? Icons.egg_rounded : Icons.hourglass_bottom_rounded,
              size: 34,
              color: isReady ? GochyaColors.secondary : GochyaColors.muted,
            ),
            const SizedBox(width: 14),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    widget.egg.origin == 'breeding'
                        ? 'Яйцо от скрещивания'
                        : 'Стартовое яйцо',
                    style: const TextStyle(fontWeight: FontWeight.w800),
                  ),
                  const SizedBox(height: 4),
                  Text(
                    isReady
                        ? 'Готово к вылуплению'
                        : 'До вылупления: '
                              '${_remainingLabel(widget.egg.incubateUntil.difference(_now))}',
                    style: const TextStyle(
                      color: GochyaColors.muted,
                      fontSize: 12,
                    ),
                  ),
                  if (_error != null) ...[
                    const SizedBox(height: 4),
                    Text(
                      _error!,
                      style: const TextStyle(
                        color: GochyaColors.warning,
                        fontSize: 12,
                      ),
                    ),
                  ],
                ],
              ),
            ),
            FilledButton.tonal(
              onPressed: !isReady || _isHatching ? null : _hatch,
              child: _isHatching
                  ? const SizedBox.square(
                      dimension: 18,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    )
                  : const Text('Вылупить'),
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
      setState(() => _now = DateTime.now());
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
          .read(breedingRepositoryProvider)
          .hatch(accessToken: widget.accessToken, eggId: widget.egg.id);
      if (!mounted) {
        return;
      }
      widget.onHatched();
    } on Object catch (error) {
      if (!mounted) {
        return;
      }
      setState(() {
        _isHatching = false;
        _now = DateTime.now();
        _error = error is ApiException && error.code == 'egg_not_ready'
            ? 'Сервер ещё завершает инкубацию.'
            : 'Не удалось вылупить питомца.';
      });
    }
  }
}

class _EmptyIncubatorCard extends StatelessWidget {
  const _EmptyIncubatorCard();

  @override
  Widget build(BuildContext context) {
    return const Card(
      child: Padding(
        padding: EdgeInsets.all(24),
        child: Column(
          children: [
            Icon(Icons.egg_outlined, size: 48),
            SizedBox(height: 10),
            Text(
              'Инкубатор пуст',
              style: TextStyle(fontWeight: FontWeight.w800),
            ),
            SizedBox(height: 6),
            Text(
              'Скрести двух взрослых питомцев, чтобы получить яйцо.',
              textAlign: TextAlign.center,
              style: TextStyle(color: GochyaColors.muted),
            ),
          ],
        ),
      ),
    );
  }
}

class _BreedingError extends StatelessWidget {
  const _BreedingError({required this.message, required this.onRetry});

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
            const Icon(Icons.egg_alt_outlined, size: 64),
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

String _remainingLabel(Duration value) {
  if (value <= Duration.zero) {
    return '0 мин.';
  }
  if (value.inMinutes < 60) {
    return '${value.inMinutes + 1} мин.';
  }
  final hours = value.inHours;
  final minutes = value.inMinutes % 60;
  return '$hours ч. $minutes мин.';
}

String _shortDate(DateTime value) {
  final local = value.toLocal();
  final day = local.day.toString().padLeft(2, '0');
  final month = local.month.toString().padLeft(2, '0');
  final hour = local.hour.toString().padLeft(2, '0');
  final minute = local.minute.toString().padLeft(2, '0');
  return '$day.$month $hour:$minute';
}
