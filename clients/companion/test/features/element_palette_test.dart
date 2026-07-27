import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gochya_companion/app/theme.dart';
import 'package:gochya_companion/core/models/technique_models.dart';
import 'package:gochya_companion/features/creatures/creature_art.dart';

/// Relative luminance per WCAG 2.x.
double _luminance(Color color) {
  double channel(double value) {
    return value <= 0.04045
        ? value / 12.92
        : math.pow((value + 0.055) / 1.055, 2.4).toDouble();
  }

  return 0.2126 * channel(color.r) +
      0.7152 * channel(color.g) +
      0.0722 * channel(color.b);
}

double _contrast(Color a, Color b) {
  final first = _luminance(a);
  final second = _luminance(b);
  final lighter = math.max(first, second);
  final darker = math.min(first, second);
  return (lighter + 0.05) / (darker + 0.05);
}

void main() {
  test('every element carries a colour of its own', () {
    // Regression guard. The tints used to be borrowed from the needs palette:
    // Water took `energy`, Earth took `hygiene`, Magma took `warning`. That
    // rebuilt the collision `ART_BIBLE.md` §5 was written to remove, so an
    // Earth pet was tinted mint directly above a mint hygiene bar.
    const forbidden = <String, Color>{
      'hunger': GochyaColors.hunger,
      'energy': GochyaColors.energy,
      'hygiene': GochyaColors.hygiene,
      'mood': GochyaColors.mood,
      'warning': GochyaColors.warning,
      'secondary': GochyaColors.secondary,
      'success': GochyaColors.success,
      'backgroundMid': GochyaColors.backgroundMid,
    };

    for (final element in CreatureElement.values) {
      for (final entry in forbidden.entries) {
        expect(
          element.tint,
          isNot(entry.value),
          reason: '${element.name} must not reuse the ${entry.key} colour',
        );
      }
    }
  });

  test('the MVP elements keep their canonical colours', () {
    expect(CreatureElement.fire.tint, GochyaElementColors.fire);
    expect(CreatureElement.water.tint, GochyaElementColors.water);
    expect(CreatureElement.earth.tint, GochyaElementColors.earth);
    expect(CreatureElement.steam.tint, GochyaElementColors.steam);
  });

  test('every tint is legible on the dark ground', () {
    // WCAG 1.4.11: non-text elements need 3:1. Mud, Eclipse and Inferno are
    // lightened for exactly this reason — their canonical values sit at
    // 1.96, 1.11 and 1.69.
    for (final element in CreatureElement.values) {
      expect(
        _contrast(element.tint, GochyaColors.background),
        greaterThanOrEqualTo(3.0),
        reason: '${element.name} is not legible on the dark background',
      );
    }
  });

  test('no two elements share a tint', () {
    final seen = <Color, String>{};
    for (final element in CreatureElement.values) {
      final clash = seen[element.tint];
      expect(
        clash,
        isNull,
        reason: '${element.name} and $clash share the same tint',
      );
      seen[element.tint] = element.name;
    }
  });

  test('water no longer collides with the energy need', () {
    // The pair this whole change started from.
    expect(CreatureElement.water.tint, isNot(GochyaColors.energy));
    expect(CreatureElement.water.tint, GochyaElementColors.water);
  });
}
