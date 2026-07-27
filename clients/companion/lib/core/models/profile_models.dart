typedef JsonMap = Map<String, dynamic>;

class PlayerProfile {
  const PlayerProfile({
    required this.id,
    required this.username,
    required this.createdAt,
    required this.streakDays,
    this.displayName,
    this.lastSeen,
    this.timezone,
    this.streakLastDay,
    this.activePetId,
  });

  factory PlayerProfile.fromJson(JsonMap json) {
    return PlayerProfile(
      id: requiredString(json, 'id'),
      username: requiredString(json, 'username'),
      displayName: optionalString(json, 'displayName'),
      createdAt: requiredDateTime(json, 'createdAt'),
      lastSeen: optionalDateTime(json, 'lastSeen'),
      timezone: optionalString(json, 'timezone'),
      streakDays: rangedInt(json, 'streakDays', min: 0),
      streakLastDay: optionalString(json, 'streakLastDay'),
      activePetId: optionalString(json, 'activePetId'),
    );
  }

  final String id;
  final String username;
  final String? displayName;
  final DateTime createdAt;
  final DateTime? lastSeen;
  final String? timezone;
  final int streakDays;
  final String? streakLastDay;
  final String? activePetId;

  String get label =>
      displayName?.trim().isNotEmpty == true ? displayName!.trim() : username;
}

class PetNeeds {
  const PetNeeds({
    required this.hunger,
    required this.energy,
    required this.hygiene,
    required this.mood,
  });

  factory PetNeeds.fromJson(JsonMap json) {
    return PetNeeds(
      hunger: rangedInt(json, 'hunger', min: 0, max: 100),
      energy: rangedInt(json, 'energy', min: 0, max: 100),
      hygiene: rangedInt(json, 'hygiene', min: 0, max: 100),
      mood: rangedInt(json, 'mood', min: 0, max: 100),
    );
  }

  final int hunger;
  final int energy;
  final int hygiene;
  final int mood;
}

class PetStats {
  const PetStats({
    required this.strength,
    required this.agility,
    required this.endurance,
    required this.focus,
  });

  factory PetStats.fromJson(JsonMap json) {
    return PetStats(
      strength: rangedInt(json, 'str', min: 0),
      agility: rangedInt(json, 'agi', min: 0),
      endurance: rangedInt(json, 'end', min: 0),
      focus: rangedInt(json, 'foc', min: 0),
    );
  }

  final int strength;
  final int agility;
  final int endurance;
  final int focus;
}

class PetSummary {
  const PetSummary({
    required this.id,
    required this.ownerId,
    required this.genome,
    required this.stage,
    required this.level,
    required this.xp,
    required this.needs,
    required this.stats,
    required this.generation,
    required this.isActive,
    required this.createdAt,
    required this.isWeak,
    required this.careRevision,
    required this.needsUpdatedAt,
    this.name,
    this.parentAId,
    this.parentBId,
    this.lastBredAt,
    this.needsZeroSince,
    this.sleepingUntil,
  });

  factory PetSummary.fromJson(JsonMap json) {
    final parentAId = optionalString(json, 'parentAId');
    final parentBId = optionalString(json, 'parentBId');
    if ((parentAId == null) != (parentBId == null)) {
      throw const FormatException('pet parents must be present as a pair');
    }

    return PetSummary(
      id: requiredString(json, 'id'),
      ownerId: requiredString(json, 'ownerId'),
      genome: requiredMap(json, 'genome'),
      name: optionalString(json, 'name'),
      stage: requiredString(json, 'stage'),
      level: rangedInt(json, 'level', min: 0),
      xp: rangedInt(json, 'xp', min: 0),
      needs: PetNeeds.fromJson(requiredMap(json, 'needs')),
      stats: PetStats.fromJson(requiredMap(json, 'stats')),
      generation: rangedInt(json, 'generation', min: 0),
      isActive: requiredBool(json, 'isActive'),
      createdAt: requiredDateTime(json, 'createdAt'),
      parentAId: parentAId,
      parentBId: parentBId,
      lastBredAt: optionalDateTime(json, 'lastBredAt'),
      needsZeroSince: optionalDateTime(json, 'needsZeroSince'),
      isWeak: requiredBool(json, 'isWeak'),
      careRevision: rangedInt(json, 'careRevision', min: 0),
      needsUpdatedAt: requiredDateTime(json, 'needsUpdatedAt'),
      sleepingUntil: optionalDateTime(json, 'sleepingUntil'),
    );
  }

  final String id;
  final String ownerId;
  final JsonMap genome;
  final String? name;
  final String stage;
  final int level;
  final int xp;
  final PetNeeds needs;
  final PetStats stats;
  final int generation;
  final bool isActive;
  final DateTime createdAt;
  final String? parentAId;
  final String? parentBId;
  final DateTime? lastBredAt;
  final DateTime? needsZeroSince;
  final bool isWeak;
  final int careRevision;
  final DateTime needsUpdatedAt;
  final DateTime? sleepingUntil;

  String get label =>
      name?.trim().isNotEmpty == true ? name!.trim() : 'Питомец';

  /// Russian name of the life stage. An unknown stage is passed through rather
  /// than guessed at: the server may add one before this client knows it.
  String get stageLabel => switch (stage.toLowerCase()) {
    'egg' => 'Яйцо',
    'baby' => 'Малыш',
    'teen' => 'Подросток',
    'adult' => 'Взрослый',
    _ => stage,
  };
}

class LineageTree {
  const LineageTree({
    required this.rootId,
    required this.maxDepth,
    required this.truncated,
    required this.nodes,
  });

  factory LineageTree.fromJson(JsonMap json) {
    final rootId = requiredString(json, 'rootId');
    final maxDepth = rangedInt(json, 'maxDepth', min: 0, max: 3);
    final rawNodes = requiredList(json, 'nodes');
    final nodes = rawNodes
        .map((value) => LineageNode.fromJson(asMap(value, 'nodes[]')))
        .toList(growable: false);
    final ids = <String>{};
    for (final node in nodes) {
      if (!ids.add(node.id)) {
        throw FormatException('duplicate lineage node ${node.id}');
      }
      if (node.ancestorDepth > maxDepth) {
        throw const FormatException('lineage node exceeds maxDepth');
      }
    }
    final rootMatches = nodes.where(
      (node) => node.id == rootId && node.ancestorDepth == 0,
    );
    if (rootMatches.length != 1) {
      throw const FormatException('lineage root is missing or invalid');
    }

    return LineageTree(
      rootId: rootId,
      maxDepth: maxDepth,
      truncated: requiredBool(json, 'truncated'),
      nodes: nodes,
    );
  }

  final String rootId;
  final int maxDepth;
  final bool truncated;
  final List<LineageNode> nodes;
}

class LineageNode {
  const LineageNode({
    required this.id,
    required this.genome,
    required this.stage,
    required this.level,
    required this.generation,
    required this.mutatedGenes,
    required this.ancestorDepth,
    this.name,
    this.parentAId,
    this.parentBId,
  });

  factory LineageNode.fromJson(JsonMap json) {
    final parentAId = optionalString(json, 'parentAId');
    final parentBId = optionalString(json, 'parentBId');
    if ((parentAId == null) != (parentBId == null)) {
      throw const FormatException('lineage parents must be present as a pair');
    }
    if (parentAId != null && parentAId == parentBId) {
      throw const FormatException('lineage parents must be distinct');
    }

    return LineageNode(
      id: requiredString(json, 'id'),
      genome: requiredMap(json, 'genome'),
      name: optionalString(json, 'name'),
      stage: requiredString(json, 'stage'),
      level: rangedInt(json, 'level', min: 0),
      generation: rangedInt(json, 'generation', min: 0),
      mutatedGenes: rangedInt(json, 'mutatedGenes', min: 0, max: 16383),
      parentAId: parentAId,
      parentBId: parentBId,
      ancestorDepth: rangedInt(json, 'ancestorDepth', min: 0, max: 3),
    );
  }

  final String id;
  final JsonMap genome;
  final String? name;
  final String stage;
  final int level;
  final int generation;
  final int mutatedGenes;
  final String? parentAId;
  final String? parentBId;
  final int ancestorDepth;

  String get label => name?.trim().isNotEmpty == true
      ? name!.trim()
      : ancestorDepth == 0
      ? 'Питомец'
      : 'Предок';
}

JsonMap asMap(Object? value, String field) {
  if (value is! Map<String, dynamic>) {
    throw FormatException('$field must be an object');
  }
  return value;
}

JsonMap requiredMap(JsonMap json, String field) => asMap(json[field], field);

List<dynamic> requiredList(JsonMap json, String field) {
  final value = json[field];
  if (value is! List<dynamic>) {
    throw FormatException('$field must be an array');
  }
  return value;
}

String requiredString(JsonMap json, String field) {
  final value = json[field];
  if (value is! String || value.trim().isEmpty) {
    throw FormatException('$field must be a non-empty string');
  }
  return value;
}

String? optionalString(JsonMap json, String field) {
  final value = json[field];
  if (value == null) {
    return null;
  }
  if (value is! String || value.trim().isEmpty) {
    throw FormatException('$field must be a non-empty string when present');
  }
  return value;
}

int rangedInt(JsonMap json, String field, {required int min, int? max}) {
  final value = json[field];
  if (value is! int || value < min || (max != null && value > max)) {
    throw FormatException('$field is outside its supported range');
  }
  return value;
}

double requiredDouble(
  JsonMap json,
  String field, {
  required double min,
  double? max,
}) {
  final value = json[field];
  if (value is! num ||
      value.isNaN ||
      value < min ||
      (max != null && value > max)) {
    throw FormatException('$field is outside its supported range');
  }
  return value.toDouble();
}

/// Reads a numeric field the server omits when it carries no value.
double optionalDouble(
  JsonMap json,
  String field, {
  required double min,
  double? max,
  double fallback = 0,
}) {
  if (json[field] == null) {
    return fallback;
  }
  return requiredDouble(json, field, min: min, max: max);
}

bool requiredBool(JsonMap json, String field) {
  final value = json[field];
  if (value is! bool) {
    throw FormatException('$field must be a boolean');
  }
  return value;
}

DateTime requiredDateTime(JsonMap json, String field) {
  final value = requiredString(json, field);
  final parsed = DateTime.tryParse(value);
  if (parsed == null) {
    throw FormatException('$field must be an ISO-8601 date-time');
  }
  return parsed;
}

DateTime? optionalDateTime(JsonMap json, String field) {
  final value = optionalString(json, field);
  if (value == null) {
    return null;
  }
  final parsed = DateTime.tryParse(value);
  if (parsed == null) {
    throw FormatException('$field must be an ISO-8601 date-time');
  }
  return parsed;
}
