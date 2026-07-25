import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../app/theme.dart';
import '../../core/identifiers/uuid_v4.dart';
import '../../core/models/technique_models.dart';
import '../../core/network/gochya_api_client.dart';
import 'technique_repository.dart';

/// Builds the five-card loadout the server uses for every match. The client
/// only submits an ordered selection; stats and outcomes stay server-side.
class LoadoutScreen extends ConsumerStatefulWidget {
  const LoadoutScreen({required this.accessToken, super.key});

  final String accessToken;

  @override
  ConsumerState<LoadoutScreen> createState() => _LoadoutScreenState();
}

class _LoadoutScreenState extends ConsumerState<LoadoutScreen> {
  final _selection = <String>[];
  var _signatureIdx = 0;
  var _isEquipping = false;
  String? _pendingIdempotencyKey;
  String? _equipError;
  var _selectionInitialised = false;

  @override
  Widget build(BuildContext context) {
    final snapshot = ref.watch(loadoutSnapshotProvider(widget.accessToken));
    return Scaffold(
      appBar: AppBar(title: const Text('Боевые карты')),
      body: snapshot.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (error, stackTrace) => _LoadoutError(
          message: _loadMessage(error),
          onRetry: () =>
              ref.invalidate(loadoutSnapshotProvider(widget.accessToken)),
        ),
        data: (value) {
          _adoptServerSelection(value);
          return RefreshIndicator(
            onRefresh: () =>
                ref.refresh(loadoutSnapshotProvider(widget.accessToken).future),
            child: _LoadoutContent(
              snapshot: value,
              selection: _selection,
              signatureIdx: _signatureIdx,
              isEquipping: _isEquipping,
              equipError: _equipError,
              onToggle: _toggleCard,
              onSignatureChanged: (index) {
                setState(() => _signatureIdx = index);
              },
              onEquip: _equip,
            ),
          );
        },
      ),
    );
  }

  /// The server loadout seeds the editor once, so a refresh never discards a
  /// selection the player is still assembling.
  void _adoptServerSelection(LoadoutSnapshot snapshot) {
    if (_selectionInitialised) {
      return;
    }
    _selectionInitialised = true;
    final loadout = snapshot.loadout;
    if (loadout == null) {
      return;
    }
    final known = loadout.cardIds
        .where((id) => snapshot.cardById(id) != null)
        .toList(growable: false);
    if (known.length != loadout.cardIds.length) {
      return;
    }
    _selection
      ..clear()
      ..addAll(known);
    _signatureIdx = loadout.signatureIdx;
  }

  void _toggleCard(String cardId) {
    setState(() {
      _equipError = null;
      final removedIndex = _selection.indexOf(cardId);
      if (removedIndex >= 0) {
        _selection.removeAt(removedIndex);
        if (_signatureIdx >= _selection.length) {
          _signatureIdx = _selection.isEmpty ? 0 : _selection.length - 1;
        }
        return;
      }
      if (_selection.length < PetLoadout.loadoutSize) {
        _selection.add(cardId);
      }
    });
  }

  Future<void> _equip() async {
    if (_selection.length != PetLoadout.loadoutSize) {
      return;
    }
    // A retry must reuse the key: equipping is idempotent per key, so a lost
    // response never produces a second, different loadout revision.
    final idempotencyKey = _pendingIdempotencyKey ?? newUuidV4();
    setState(() {
      _isEquipping = true;
      _pendingIdempotencyKey = idempotencyKey;
      _equipError = null;
    });
    try {
      await ref
          .read(techniqueRepositoryProvider)
          .equip(
            accessToken: widget.accessToken,
            cardIds: List.unmodifiable(_selection),
            signatureIdx: _signatureIdx,
            idempotencyKey: idempotencyKey,
          );
      if (!mounted) {
        return;
      }
      setState(() {
        _isEquipping = false;
        _pendingIdempotencyKey = null;
      });
      ref.invalidate(loadoutSnapshotProvider(widget.accessToken));
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(const SnackBar(content: Text('Лоадаут сохранён')));
    } on Object catch (error) {
      if (!mounted) {
        return;
      }
      setState(() {
        _isEquipping = false;
        _equipError = _equipMessage(error);
      });
    }
  }

  String _equipMessage(Object error) {
    if (error is! ApiException) {
      return 'Сервер не подтвердил лоадаут. Повтори — запрос идемпотентный.';
    }
    return switch (error.code) {
      'active_pet_required' => 'Сначала нужен активный питомец.',
      'loadout_cards_invalid' =>
        'В лоадауте должны быть пять твоих карт. Обнови список.',
      'loadout_invalid' => 'Подпись должна указывать на одну из пяти карт.',
      'idempotency_conflict' =>
        'Этот запрос уже применён с другим составом. Обнови экран.',
      'request_timeout' || 'network_error' =>
        'Сервер не подтвердил лоадаут. Повтори — запрос идемпотентный.',
      _ => 'Не удалось сохранить лоадаут.',
    };
  }

  String _loadMessage(Object error) {
    if (error is ApiException &&
        (error.code == 'request_timeout' || error.code == 'network_error')) {
      return 'Карты доступны только с сервера. Проверь соединение.';
    }
    return 'Не удалось загрузить коллекцию карт.';
  }
}

class _LoadoutContent extends StatelessWidget {
  const _LoadoutContent({
    required this.snapshot,
    required this.selection,
    required this.signatureIdx,
    required this.isEquipping,
    required this.equipError,
    required this.onToggle,
    required this.onSignatureChanged,
    required this.onEquip,
  });

  final LoadoutSnapshot snapshot;
  final List<String> selection;
  final int signatureIdx;
  final bool isEquipping;
  final String? equipError;
  final ValueChanged<String> onToggle;
  final ValueChanged<int> onSignatureChanged;
  final Future<void> Function() onEquip;

  @override
  Widget build(BuildContext context) {
    final isComplete = selection.length == PetLoadout.loadoutSize;
    return ListView(
      physics: const AlwaysScrollableScrollPhysics(),
      padding: const EdgeInsets.fromLTRB(20, 12, 20, 32),
      children: [
        _LoadoutStatusCard(snapshot: snapshot, selected: selection.length),
        if (selection.isNotEmpty) ...[
          const SizedBox(height: 16),
          _SignaturePicker(
            snapshot: snapshot,
            selection: selection,
            signatureIdx: signatureIdx,
            onSignatureChanged: onSignatureChanged,
          ),
        ],
        if (equipError != null) ...[
          const SizedBox(height: 12),
          Text(
            equipError!,
            textAlign: TextAlign.center,
            style: TextStyle(color: Theme.of(context).colorScheme.error),
          ),
        ],
        const SizedBox(height: 16),
        FilledButton.icon(
          onPressed: !isComplete || isEquipping ? null : onEquip,
          icon: isEquipping
              ? const SizedBox.square(
                  dimension: 18,
                  child: CircularProgressIndicator(strokeWidth: 2),
                )
              : const Icon(Icons.shield_moon_outlined),
          label: Text(
            isComplete
                ? 'Экипировать пять карт'
                : 'Выбрано ${selection.length} из ${PetLoadout.loadoutSize}',
          ),
        ),
        const SizedBox(height: 24),
        Text(
          'Коллекция · ${snapshot.cards.length}',
          style: Theme.of(
            context,
          ).textTheme.titleLarge?.copyWith(fontWeight: FontWeight.w800),
        ),
        const SizedBox(height: 12),
        if (snapshot.cards.isEmpty)
          const _EmptyCollectionCard()
        else
          for (final card in snapshot.cards) ...[
            _TechniqueCardTile(
              card: card,
              slot: selection.indexOf(card.id),
              selectionFull: isComplete,
              onToggle: () => onToggle(card.id),
            ),
            const SizedBox(height: 10),
          ],
      ],
    );
  }
}

class _LoadoutStatusCard extends StatelessWidget {
  const _LoadoutStatusCard({required this.snapshot, required this.selected});

  final LoadoutSnapshot snapshot;
  final int selected;

  @override
  Widget build(BuildContext context) {
    final loadout = snapshot.loadout;
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
                      ? GochyaColors.muted
                      : GochyaColors.success,
                ),
                const SizedBox(width: 10),
                Expanded(
                  child: Text(
                    loadout == null
                        ? 'Лоадаут ещё не собран'
                        : 'Лоадаут активен · ревизия ${loadout.revision}',
                    style: Theme.of(context).textTheme.titleMedium?.copyWith(
                      fontWeight: FontWeight.w800,
                    ),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 10),
            Text(
              loadout == null
                  ? 'Для боя сервер требует пять карт и одну signature-позицию.'
                  : 'Обновлён ${_dateLabel(loadout.updatedAt)}. '
                        'Бой всегда считается по серверной ревизии.',
              style: const TextStyle(color: GochyaColors.muted),
            ),
            if (!snapshot.canEquip) ...[
              const SizedBox(height: 10),
              Text(
                'Нужно минимум ${PetLoadout.loadoutSize} карт: добывай их '
                'ежедневной активностью и победами в casual.',
                style: const TextStyle(color: GochyaColors.warning),
              ),
            ],
          ],
        ),
      ),
    );
  }
}

class _SignaturePicker extends StatelessWidget {
  const _SignaturePicker({
    required this.snapshot,
    required this.selection,
    required this.signatureIdx,
    required this.onSignatureChanged,
  });

  final LoadoutSnapshot snapshot;
  final List<String> selection;
  final int signatureIdx;
  final ValueChanged<int> onSignatureChanged;

  @override
  Widget build(BuildContext context) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              'Signature-карта',
              style: Theme.of(
                context,
              ).textTheme.titleMedium?.copyWith(fontWeight: FontWeight.w800),
            ),
            const SizedBox(height: 12),
            Wrap(
              spacing: 8,
              runSpacing: 8,
              children: [
                for (var index = 0; index < selection.length; index++)
                  ChoiceChip(
                    selected: index == signatureIdx,
                    onSelected: (_) => onSignatureChanged(index),
                    label: Text(
                      snapshot.cardById(selection[index])?.type.label ??
                          'Слот ${index + 1}',
                    ),
                  ),
              ],
            ),
          ],
        ),
      ),
    );
  }
}

class _TechniqueCardTile extends StatelessWidget {
  const _TechniqueCardTile({
    required this.card,
    required this.slot,
    required this.selectionFull,
    required this.onToggle,
  });

  final TechniqueCardSummary card;
  final int slot;
  final bool selectionFull;
  final VoidCallback onToggle;

  @override
  Widget build(BuildContext context) {
    final isSelected = slot >= 0;
    final canSelect = isSelected || !selectionFull;
    return Card(
      child: InkWell(
        borderRadius: BorderRadius.circular(20),
        onTap: canSelect ? onToggle : null,
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Row(
            children: [
              Container(
                width: 46,
                height: 46,
                decoration: BoxDecoration(
                  color: card.rarity.color.withValues(alpha: 0.2),
                  borderRadius: BorderRadius.circular(14),
                ),
                alignment: Alignment.center,
                child: Text(
                  isSelected ? '${slot + 1}' : card.type.label.substring(0, 1),
                  style: TextStyle(
                    fontWeight: FontWeight.w900,
                    color: card.rarity.color,
                  ),
                ),
              ),
              const SizedBox(width: 14),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      card.label,
                      style: Theme.of(context).textTheme.titleMedium?.copyWith(
                        fontWeight: FontWeight.w800,
                      ),
                    ),
                    const SizedBox(height: 4),
                    Text(
                      '${card.rarity.label} · урон '
                      '${card.baseDamage.toStringAsFixed(1)} · '
                      'стамина ${card.staminaCost}',
                      style: const TextStyle(
                        color: GochyaColors.muted,
                        fontSize: 12,
                      ),
                    ),
                    if (card.effect != TechniqueEffect.none) ...[
                      const SizedBox(height: 2),
                      Text(
                        '${card.effect.label} '
                        '${card.effectValue.toStringAsFixed(2)}',
                        style: const TextStyle(
                          color: GochyaColors.secondary,
                          fontSize: 12,
                        ),
                      ),
                    ],
                  ],
                ),
              ),
              Checkbox(
                value: isSelected,
                onChanged: canSelect ? (_) => onToggle() : null,
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _EmptyCollectionCard extends StatelessWidget {
  const _EmptyCollectionCard();

  @override
  Widget build(BuildContext context) {
    return const Card(
      child: Padding(
        padding: EdgeInsets.all(24),
        child: Column(
          children: [
            Icon(Icons.style_outlined, size: 52),
            SizedBox(height: 12),
            Text(
              'Карт пока нет',
              style: TextStyle(fontWeight: FontWeight.w800),
            ),
            SizedBox(height: 6),
            Text(
              'Телефон получает карты за дневную активность и первую победу '
              'дня. Запись ударов доступна только на часах.',
              textAlign: TextAlign.center,
              style: TextStyle(color: GochyaColors.muted),
            ),
          ],
        ),
      ),
    );
  }
}

class _LoadoutError extends StatelessWidget {
  const _LoadoutError({required this.message, required this.onRetry});

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
            const Icon(Icons.style_outlined, size: 64),
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

String _dateLabel(DateTime value) {
  final local = value.toLocal();
  final day = local.day.toString().padLeft(2, '0');
  final month = local.month.toString().padLeft(2, '0');
  final hour = local.hour.toString().padLeft(2, '0');
  final minute = local.minute.toString().padLeft(2, '0');
  return '$day.$month в $hour:$minute';
}

extension TechniqueRarityStyle on TechniqueRarity {
  Color get color => switch (this) {
    TechniqueRarity.common => GochyaColors.muted,
    TechniqueRarity.uncommon => GochyaColors.hygiene,
    TechniqueRarity.rare => GochyaColors.energy,
    TechniqueRarity.epic => GochyaColors.primary,
    TechniqueRarity.legendary => GochyaColors.secondary,
    TechniqueRarity.mythic => GochyaColors.warning,
  };
}
