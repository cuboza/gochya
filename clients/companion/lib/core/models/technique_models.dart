import 'profile_models.dart';

/// Stable protocol IDs mirrored from `core/src/technique.rs`.
enum TechniqueType {
  jab(0, 'Джеб'),
  hook(1, 'Хук'),
  uppercut(2, 'Апперкот'),
  cross(3, 'Кросс'),
  kick(4, 'Кик'),
  elbow(5, 'Локоть'),
  block(6, 'Блок');

  const TechniqueType(this.apiValue, this.label);

  factory TechniqueType.fromApi(int value) {
    return values.firstWhere(
      (type) => type.apiValue == value,
      orElse: () => throw FormatException('unsupported technique type $value'),
    );
  }

  final int apiValue;
  final String label;
}

enum TechniqueRarity {
  common(0, 'Common'),
  uncommon(1, 'Uncommon'),
  rare(2, 'Rare'),
  epic(3, 'Epic'),
  legendary(4, 'Legendary'),
  mythic(5, 'Mythic');

  const TechniqueRarity(this.apiValue, this.label);

  factory TechniqueRarity.fromApi(int value) {
    return values.firstWhere(
      (rarity) => rarity.apiValue == value,
      orElse: () => throw FormatException('unsupported rarity $value'),
    );
  }

  final int apiValue;
  final String label;
}

enum TechniqueEffect {
  none(0, 'Без эффекта'),
  stun(1, 'Оглушение'),
  bleed(2, 'Кровотечение'),
  crit(3, 'Крит'),
  slow(4, 'Замедление'),
  heal(5, 'Лечение');

  const TechniqueEffect(this.apiValue, this.label);

  factory TechniqueEffect.fromApi(int value) {
    return values.firstWhere(
      (effect) => effect.apiValue == value,
      orElse: () => throw FormatException('unsupported effect $value'),
    );
  }

  final int apiValue;
  final String label;
}

/// Stable protocol IDs mirrored from `core/src/genome.rs`.
enum CreatureElement {
  fire(0, 'Огонь'),
  water(1, 'Вода'),
  earth(2, 'Земля'),
  air(3, 'Воздух'),
  light(4, 'Свет'),
  dark(5, 'Тьма'),
  arcane(6, 'Аркана'),
  steam(7, 'Пар'),
  magma(8, 'Магма'),
  storm(9, 'Шторм'),
  mud(10, 'Грязь'),
  smoke(11, 'Дым'),
  sand(12, 'Песок'),
  eclipse(13, 'Затмение'),
  inferno(14, 'Инферно'),
  prism(15, 'Призма'),
  crystal(16, 'Кристалл');

  const CreatureElement(this.apiValue, this.label);

  factory CreatureElement.fromApi(int value) {
    return values.firstWhere(
      (element) => element.apiValue == value,
      orElse: () => throw FormatException('unsupported element $value'),
    );
  }

  final int apiValue;
  final String label;
}

class TechniqueCardSummary {
  const TechniqueCardSummary({
    required this.id,
    required this.ownerId,
    required this.type,
    required this.element,
    required this.rarity,
    required this.baseDamage,
    required this.speed,
    required this.staminaCost,
    required this.critChance,
    required this.effect,
    required this.effectValue,
    required this.quality,
    required this.createdAt,
  });

  factory TechniqueCardSummary.fromJson(JsonMap json) {
    final effect = TechniqueEffect.fromApi(
      rangedInt(json, 'effect', min: 0, max: 5),
    );
    final effectValue = optionalDouble(json, 'effectValue', min: 0, max: 1000);
    if (effect == TechniqueEffect.none && effectValue != 0) {
      throw const FormatException('technique effect value is inconsistent');
    }
    return TechniqueCardSummary(
      id: requiredString(json, 'id'),
      ownerId: requiredString(json, 'ownerId'),
      type: TechniqueType.fromApi(rangedInt(json, 'type', min: 0, max: 6)),
      element: CreatureElement.fromApi(
        rangedInt(json, 'element', min: 0, max: 16),
      ),
      rarity: TechniqueRarity.fromApi(
        rangedInt(json, 'rarity', min: 0, max: 5),
      ),
      baseDamage: requiredDouble(json, 'baseDamage', min: 0, max: 10000),
      speed: requiredDouble(json, 'speed', min: 0, max: 10000),
      staminaCost: rangedInt(json, 'staminaCost', min: 0, max: 65535),
      critChance: requiredDouble(json, 'critChance', min: 0, max: 1),
      effect: effect,
      effectValue: effectValue,
      quality: rangedInt(json, 'quality', min: 0, max: 100),
      createdAt: requiredDateTime(json, 'createdAt'),
    );
  }

  final String id;
  final String ownerId;
  final TechniqueType type;
  final CreatureElement element;
  final TechniqueRarity rarity;
  final double baseDamage;
  final double speed;
  final int staminaCost;
  final double critChance;
  final TechniqueEffect effect;
  final double effectValue;
  final int quality;
  final DateTime createdAt;

  String get label => '${type.label} · ${element.label}';
}

class TechniquePage {
  const TechniquePage({required this.items, this.nextCursor});

  factory TechniquePage.fromJson(JsonMap json) {
    final rawItems = requiredList(json, 'items');
    if (rawItems.length > 100) {
      throw const FormatException('technique page is too large');
    }
    final items = rawItems
        .map((value) => TechniqueCardSummary.fromJson(asMap(value, 'items[]')))
        .toList(growable: false);
    if (items.map((item) => item.id).toSet().length != items.length) {
      throw const FormatException('technique page contains duplicate cards');
    }
    final nextCursor = optionalString(json, 'next_cursor');
    if (nextCursor != null && (items.isEmpty || nextCursor.length > 512)) {
      throw const FormatException('technique cursor is invalid');
    }
    return TechniquePage(
      items: List.unmodifiable(items),
      nextCursor: nextCursor,
    );
  }

  final List<TechniqueCardSummary> items;
  final String? nextCursor;
}

/// Server-authoritative five-card loadout of the active pet.
class PetLoadout {
  const PetLoadout({
    required this.petId,
    required this.cardIds,
    required this.signatureIdx,
    required this.revision,
    required this.updatedAt,
  });

  factory PetLoadout.fromJson(JsonMap json) {
    final cardIds = requiredList(json, 'cardIds')
        .map((value) {
          if (value is! String || value.trim().isEmpty) {
            throw const FormatException('cardIds[] must be a non-empty string');
          }
          return value;
        })
        .toList(growable: false);
    if (cardIds.length != loadoutSize ||
        cardIds.toSet().length != cardIds.length) {
      throw const FormatException('loadout must hold five distinct cards');
    }
    return PetLoadout(
      petId: requiredString(json, 'petId'),
      cardIds: List.unmodifiable(cardIds),
      signatureIdx: rangedInt(
        json,
        'signatureIdx',
        min: 0,
        max: loadoutSize - 1,
      ),
      revision: rangedInt(json, 'revision', min: 0),
      updatedAt: requiredDateTime(json, 'updatedAt'),
    );
  }

  static const loadoutSize = 5;

  final String petId;
  final List<String> cardIds;
  final int signatureIdx;
  final int revision;
  final DateTime updatedAt;

  String get signatureCardId => cardIds[signatureIdx];
}
