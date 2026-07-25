import 'profile_models.dart';

enum StarterElement {
  fire('fire', 'Огонь'),
  water('water', 'Вода'),
  earth('earth', 'Земля');

  const StarterElement(this.apiValue, this.label);

  factory StarterElement.fromApiValue(String value) {
    return StarterElement.values.firstWhere(
      (element) => element.apiValue == value,
      orElse: () => throw FormatException('unsupported starter element $value'),
    );
  }

  final String apiValue;
  final String label;
}

class AgeGateResult {
  const AgeGateResult({
    required this.status,
    required this.coppaRestricted,
    required this.recordedAt,
  });

  factory AgeGateResult.fromJson(JsonMap json) {
    final status = requiredString(json, 'status');
    if (status != 'eligible' && status != 'parental_consent_required') {
      throw FormatException('unsupported age gate status $status');
    }
    final restricted = requiredBool(json, 'coppaRestricted');
    if ((status == 'eligible') == restricted) {
      throw const FormatException('age gate status is inconsistent');
    }
    return AgeGateResult(
      status: status,
      coppaRestricted: restricted,
      recordedAt: requiredDateTime(json, 'recordedAt'),
    );
  }

  final String status;
  final bool coppaRestricted;
  final DateTime recordedAt;

  bool get isEligible => status == 'eligible' && !coppaRestricted;
}

class StarterEggResult {
  const StarterEggResult({
    required this.eggId,
    required this.element,
    required this.incubateUntil,
  });

  factory StarterEggResult.fromJson(JsonMap json) {
    return StarterEggResult(
      eggId: requiredString(json, 'eggId'),
      element: StarterElement.fromApiValue(requiredString(json, 'element')),
      incubateUntil: requiredDateTime(json, 'incubateUntil'),
    );
  }

  final String eggId;
  final StarterElement element;
  final DateTime incubateUntil;
}

class EggSummary {
  const EggSummary({
    required this.id,
    required this.ownerId,
    required this.origin,
    required this.genome,
    required this.incubateUntil,
    required this.mutatedGenes,
    required this.createdAt,
    this.parentAId,
    this.parentBId,
  });

  factory EggSummary.fromJson(JsonMap json) {
    final origin = requiredString(json, 'origin');
    if (origin != 'starter' && origin != 'breeding') {
      throw FormatException('unsupported egg origin $origin');
    }
    final parentAId = optionalString(json, 'parentAId');
    final parentBId = optionalString(json, 'parentBId');
    if ((parentAId == null) != (parentBId == null)) {
      throw const FormatException('egg parents must be present as a pair');
    }
    if (origin == 'starter' && parentAId != null) {
      throw const FormatException('starter egg must not have parents');
    }
    if (origin == 'breeding' && parentAId == null) {
      throw const FormatException('breeding egg must have parents');
    }

    return EggSummary(
      id: requiredString(json, 'id'),
      ownerId: requiredString(json, 'ownerId'),
      origin: origin,
      genome: requiredMap(json, 'genome'),
      parentAId: parentAId,
      parentBId: parentBId,
      incubateUntil: requiredDateTime(json, 'incubateUntil'),
      mutatedGenes: rangedInt(json, 'mutatedGenes', min: 0, max: 16383),
      createdAt: requiredDateTime(json, 'createdAt'),
    );
  }

  final String id;
  final String ownerId;
  final String origin;
  final JsonMap genome;
  final String? parentAId;
  final String? parentBId;
  final DateTime incubateUntil;
  final int mutatedGenes;
  final DateTime createdAt;

  bool isReadyAt(DateTime now) => !incubateUntil.isAfter(now);
}

class HatchedPet {
  const HatchedPet({
    required this.id,
    required this.ownerId,
    required this.stage,
    required this.isActive,
  });

  factory HatchedPet.fromJson(JsonMap json) {
    final parentAId = optionalString(json, 'parentAId');
    final parentBId = optionalString(json, 'parentBId');
    if ((parentAId == null) != (parentBId == null)) {
      throw const FormatException(
        'hatched pet parents must be present as a pair',
      );
    }
    requiredMap(json, 'genome');
    rangedInt(json, 'level', min: 0);
    rangedInt(json, 'xp', min: 0);
    PetNeeds.fromJson(requiredMap(json, 'needs'));
    PetStats.fromJson(requiredMap(json, 'stats'));
    rangedInt(json, 'generation', min: 0);
    requiredDateTime(json, 'createdAt');
    requiredBool(json, 'isWeak');

    return HatchedPet(
      id: requiredString(json, 'id'),
      ownerId: requiredString(json, 'ownerId'),
      stage: requiredString(json, 'stage'),
      isActive: requiredBool(json, 'isActive'),
    );
  }

  final String id;
  final String ownerId;
  final String stage;
  final bool isActive;
}
