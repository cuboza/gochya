import 'package:flutter/material.dart';

import '../../app/theme.dart';
import '../../core/models/technique_models.dart';
import '../creatures/creature_art.dart';
import 'technique_content.dart';
import 'technique_glyph.dart';

/// A Technique Card face: the strike's own motion signature, framed in its
/// rarity colour, with the element it was made by and the server's numbers.
class TechniqueCardFace extends StatelessWidget {
  const TechniqueCardFace({
    required this.card,
    this.slot = -1,
    this.trailing,
    this.onTap,
    super.key,
  });

  final TechniqueCardSummary card;

  /// Position in the loadout, or a negative value when the card is unequipped.
  final int slot;
  final Widget? trailing;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final frame = card.rarity.frameColor;
    final tint = card.element.tint;
    return Semantics(
      label:
          '${card.type.label}, ${card.element.label}, ${card.rarity.label}'
          '${slot >= 0 ? ', слот ${slot + 1}' : ''}',
      child: DecoratedBox(
        decoration: BoxDecoration(
          borderRadius: BorderRadius.circular(20),
          boxShadow: card.rarity.glow == 0
              ? null
              : [
                  BoxShadow(
                    color: frame.withValues(alpha: 0.3),
                    blurRadius: card.rarity.glow,
                    spreadRadius: card.rarity.glow / 10,
                  ),
                ],
        ),
        child: Material(
          color: GochyaColors.backgroundMid,
          borderRadius: BorderRadius.circular(20),
          child: InkWell(
            borderRadius: BorderRadius.circular(20),
            onTap: onTap,
            child: Container(
              decoration: BoxDecoration(
                borderRadius: BorderRadius.circular(20),
                border: Border.all(color: frame, width: card.rarity.frameWidth),
              ),
              padding: const EdgeInsets.all(14),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      _GlyphPlate(card: card, slot: slot, tint: tint),
                      const SizedBox(width: 14),
                      Expanded(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Text(
                              card.type.label,
                              style: Theme.of(context).textTheme.titleMedium
                                  ?.copyWith(fontWeight: FontWeight.w900),
                            ),
                            const SizedBox(height: 2),
                            Row(
                              children: [
                                Text(
                                  card.rarity.label.toUpperCase(),
                                  style: TextStyle(
                                    color: card.rarity.labelColor,
                                    fontWeight: FontWeight.w800,
                                    fontSize: 11,
                                    letterSpacing: 1.1,
                                  ),
                                ),
                                const SizedBox(width: 8),
                                Text(
                                  '· ${card.element.label}',
                                  style: const TextStyle(
                                    color: GochyaColors.muted,
                                    fontSize: 11,
                                  ),
                                ),
                              ],
                            ),
                            const SizedBox(height: 8),
                            Text(
                              card.type.description,
                              style: const TextStyle(
                                fontSize: 12,
                                height: 1.35,
                              ),
                            ),
                          ],
                        ),
                      ),
                      ?trailing,
                    ],
                  ),
                  const SizedBox(height: 12),
                  Text(
                    card.type.lore,
                    style: const TextStyle(
                      color: GochyaColors.muted,
                      fontSize: 12,
                      fontStyle: FontStyle.italic,
                      height: 1.3,
                    ),
                  ),
                  const SizedBox(height: 12),
                  Divider(height: 1, color: frame.withValues(alpha: 0.28)),
                  const SizedBox(height: 10),
                  _StatStrip(card: card),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}

class _GlyphPlate extends StatelessWidget {
  const _GlyphPlate({
    required this.card,
    required this.slot,
    required this.tint,
  });

  final TechniqueCardSummary card;
  final int slot;
  final Color tint;

  static const _size = 72.0;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: _size,
      height: _size,
      child: Stack(
        clipBehavior: Clip.none,
        children: [
          Container(
            width: _size,
            height: _size,
            decoration: BoxDecoration(
              gradient: LinearGradient(
                begin: Alignment.topLeft,
                end: Alignment.bottomRight,
                colors: [
                  tint.withValues(alpha: 0.28),
                  GochyaColors.background.withValues(alpha: 0.7),
                ],
              ),
              borderRadius: BorderRadius.circular(18),
            ),
            alignment: Alignment.center,
            child: TechniqueGlyph(
              type: card.type,
              color: card.rarity.frameColor,
              size: _size * 0.78,
            ),
          ),
          // The species that produced the card stays visible as a small badge.
          Positioned(
            right: -4,
            bottom: -4,
            child: SizedBox.square(
              dimension: 26,
              child: CreatureAvatar(element: card.element, size: 26),
            ),
          ),
          if (slot >= 0)
            Positioned(
              top: -6,
              left: -6,
              child: Container(
                width: 22,
                height: 22,
                alignment: Alignment.center,
                decoration: BoxDecoration(
                  color: card.rarity.frameColor,
                  shape: BoxShape.circle,
                ),
                child: Text(
                  '${slot + 1}',
                  style: const TextStyle(
                    fontSize: 12,
                    fontWeight: FontWeight.w900,
                    color: GochyaColors.background,
                  ),
                ),
              ),
            ),
        ],
      ),
    );
  }
}

class _StatStrip extends StatelessWidget {
  const _StatStrip({required this.card});

  final TechniqueCardSummary card;

  @override
  Widget build(BuildContext context) {
    return Wrap(
      spacing: 14,
      runSpacing: 6,
      children: [
        _Stat(label: 'урон', value: card.baseDamage.toStringAsFixed(1)),
        _Stat(label: 'стамина', value: '${card.staminaCost}'),
        _Stat(label: 'скорость', value: card.speed.toStringAsFixed(0)),
        _Stat(
          label: 'крит',
          value: '${(card.critChance * 100).toStringAsFixed(0)}%',
        ),
        _Stat(
          label: 'тип',
          value: '×${card.type.typeMultiplier.toStringAsFixed(2)}',
        ),
        if (card.effect != TechniqueEffect.none)
          _Stat(
            label: card.effect.label.toLowerCase(),
            value: card.effectValue.toStringAsFixed(2),
            highlight: true,
          ),
      ],
    );
  }
}

class _Stat extends StatelessWidget {
  const _Stat({
    required this.label,
    required this.value,
    this.highlight = false,
  });

  final String label;
  final String value;
  final bool highlight;

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(
          '$label ',
          style: const TextStyle(color: GochyaColors.muted, fontSize: 11),
        ),
        Text(
          value,
          style: TextStyle(
            fontWeight: FontWeight.w800,
            fontSize: 12,
            color: highlight ? GochyaColors.secondary : null,
          ),
        ),
      ],
    );
  }
}
