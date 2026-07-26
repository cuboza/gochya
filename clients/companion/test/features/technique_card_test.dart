import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gochya_companion/app/theme.dart';
import 'package:gochya_companion/core/models/technique_models.dart';
import 'package:gochya_companion/features/techniques/technique_card.dart';
import 'package:gochya_companion/features/techniques/technique_content.dart';

void main() {
  test('every strike carries its own description, lore and weight', () {
    final descriptions = <String>{};
    final lores = <String>{};
    for (final type in TechniqueType.values) {
      expect(type.description, isNotEmpty, reason: type.name);
      expect(type.lore, isNotEmpty, reason: type.name);
      descriptions.add(type.description);
      lores.add(type.lore);
    }
    expect(descriptions, hasLength(TechniqueType.values.length));
    expect(lores, hasLength(TechniqueType.values.length));
  });

  test('type multipliers mirror core/src/technique.rs', () {
    expect(TechniqueType.jab.typeMultiplier, 0.9);
    expect(TechniqueType.hook.typeMultiplier, 1.0);
    expect(TechniqueType.uppercut.typeMultiplier, 1.15);
    expect(TechniqueType.cross.typeMultiplier, 1.1);
    expect(TechniqueType.kick.typeMultiplier, 1.2);
    expect(TechniqueType.elbow.typeMultiplier, 1.1);
    expect(TechniqueType.block.typeMultiplier, 0.3);
  });

  test('rarity frames follow the ART_BIBLE ladder', () {
    // Grey at the bottom through purple, orange and the iridescent top tier.
    expect(TechniqueRarity.common.frameColor, const Color(0xFF8A8FA3));
    expect(TechniqueRarity.uncommon.frameColor, const Color(0xFF4CAF6E));
    expect(TechniqueRarity.rare.frameColor, const Color(0xFF3D8BFD));
    expect(TechniqueRarity.epic.frameColor, const Color(0xFFA855F7));
    expect(TechniqueRarity.legendary.frameColor, const Color(0xFFFF9F1C));
    expect(TechniqueRarity.mythic.frameColor, const Color(0xFFF5F0FF));

    // Frames are a scale: colour and glow both rise with rarity.
    final frames = TechniqueRarity.values
        .map((rarity) => rarity.frameColor)
        .toSet();
    expect(frames, hasLength(TechniqueRarity.values.length));
    for (var index = 1; index < TechniqueRarity.values.length; index++) {
      expect(
        TechniqueRarity.values[index].glow,
        greaterThan(TechniqueRarity.values[index - 1].glow),
      );
    }
    expect(TechniqueRarity.common.glow, 0);
  });

  testWidgets('the card face shows the strike, not just the element', (
    tester,
  ) async {
    await tester.binding.setSurfaceSize(const Size(600, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));
    await tester.pumpWidget(
      MaterialApp(
        theme: buildGochyaTheme(),
        home: Scaffold(
          body: TechniqueCardFace(card: _card(TechniqueType.uppercut), slot: 2),
        ),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('Апперкот'), findsOneWidget);
    expect(find.text('EPIC'), findsOneWidget);
    expect(find.text(TechniqueType.uppercut.description), findsOneWidget);
    expect(find.text(TechniqueType.uppercut.lore), findsOneWidget);
    expect(find.text('×1.15'), findsOneWidget);
    // Slot badge is one-based for the player.
    expect(find.text('3'), findsOneWidget);
  });
}

TechniqueCardSummary _card(TechniqueType type) {
  return TechniqueCardSummary(
    id: 'card-1',
    ownerId: 'player-1',
    type: type,
    element: CreatureElement.water,
    rarity: TechniqueRarity.epic,
    baseDamage: 24,
    speed: 50,
    staminaCost: 14,
    critChance: 0.08,
    effect: TechniqueEffect.none,
    effectValue: 0,
    quality: 72,
    createdAt: DateTime.parse('2026-07-24T10:00:00Z'),
  );
}
