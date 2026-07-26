import 'package:flutter/material.dart';

import '../../app/theme.dart';
import '../../core/models/technique_models.dart';

/// Presentation copy and framing rules for Technique Cards.
///
/// The numbers a card shows always come from the server. Everything here is
/// flavour and framing: what the strike is, how it reads, and how strong its
/// frame glows.
extension TechniqueTypeContent on TechniqueType {
  /// One line on what the strike does mechanically. `typeMultiplier` mirrors
  /// `type_multiplier` in `core/src/technique.rs`; it is shown, never applied.
  String get description => switch (this) {
    TechniqueType.jab =>
      'Короткий прямой передней рукой. Дёшев по стамине '
          'и почти всегда успевает первым.',
    TechniqueType.hook =>
      'Боковая дуга по корпусу или челюсти. Заходит мимо '
          'прямой защиты.',
    TechniqueType.uppercut =>
      'Подъём снизу вверх на короткой дистанции. '
          'Один из самых тяжёлых ударов рукой.',
    TechniqueType.cross =>
      'Прямой дальней рукой со скруткой корпуса. Длинный '
          'и пробивной.',
    TechniqueType.kick =>
      'Удар ногой с максимальной массой. Самый сильный '
          'и самый дорогой по стамине.',
    TechniqueType.elbow =>
      'Ближний удар локтем. Жёсткий, рассекающий, '
          'работает вплотную.',
    TechniqueType.block =>
      'Защитная форма. Почти не наносит урона, но гасит '
          'чужой.',
  };

  /// Flavour line. Never states a rule the core does not implement.
  String get lore => switch (this) {
    TechniqueType.jab =>
      '«Первый удар, которому учат в любом зале: не самый '
          'сильный — просто всегда первый».',
    TechniqueType.hook => '«Приходит оттуда, куда не смотрят».',
    TechniqueType.uppercut =>
      '«Земля толкает кулак вверх. Тело только '
          'передаёт её волю».',
    TechniqueType.cross => '«Плечо, бедро и пятка стреляют одним движением».',
    TechniqueType.kick =>
      '«Длинная дистанция прощает ошибку в расчёте, '
          'но не в равновесии».',
    TechniqueType.elbow => '«На дистанции вздоха кость становится лезвием».',
    TechniqueType.block => '«Иногда лучший приём — не пропустить чужой».',
  };

  /// Damage weight of the strike type, from `CORE_FORMULAS.md`.
  double get typeMultiplier => switch (this) {
    TechniqueType.jab => 0.9,
    TechniqueType.hook => 1.0,
    TechniqueType.uppercut => 1.15,
    TechniqueType.cross => 1.1,
    TechniqueType.kick => 1.2,
    TechniqueType.elbow => 1.1,
    TechniqueType.block => 0.3,
  };
}

extension TechniqueRarityStyle on TechniqueRarity {
  Color get frameColor => switch (this) {
    TechniqueRarity.common => GochyaRarityColors.common,
    TechniqueRarity.uncommon => GochyaRarityColors.uncommon,
    TechniqueRarity.rare => GochyaRarityColors.rare,
    TechniqueRarity.epic => GochyaRarityColors.epic,
    TechniqueRarity.legendary => GochyaRarityColors.legendary,
    TechniqueRarity.mythic => GochyaRarityColors.mythic,
  };

  /// Frame glow strength, matching the ladder in `ART_BIBLE.md` §3.3.
  double get glow => switch (this) {
    TechniqueRarity.common => 0,
    TechniqueRarity.uncommon => 4,
    TechniqueRarity.rare => 8,
    TechniqueRarity.epic => 14,
    TechniqueRarity.legendary => 20,
    TechniqueRarity.mythic => 26,
  };

  double get frameWidth => switch (this) {
    TechniqueRarity.common || TechniqueRarity.uncommon => 1.5,
    TechniqueRarity.rare || TechniqueRarity.epic => 2,
    _ => 2.5,
  };

  /// Epic fails the 4.5:1 text threshold, so labels fall back to a readable
  /// colour while the frame keeps the canonical hue.
  Color get labelColor =>
      this == TechniqueRarity.epic ? GochyaColors.primary : frameColor;
}
