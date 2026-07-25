import 'profile_models.dart';

enum CareOperation {
  feed('feed'),
  clean('clean'),
  play('play'),
  sleep('sleep');

  const CareOperation(this.apiValue);

  final String apiValue;
}

class CareIntent {
  const CareIntent({
    required this.operationId,
    required this.operation,
    required this.clientWallTime,
    required this.clientMonotonicOffsetMs,
    this.itemId,
  });

  final String operationId;
  final CareOperation operation;
  final String? itemId;
  final DateTime clientWallTime;
  final int clientMonotonicOffsetMs;

  JsonMap toJson({required String petId, required int baseRevision}) {
    final allowedItem = switch (operation) {
      CareOperation.feed =>
        itemId == 'apple' || itemId == 'steak' || itemId == 'energy_drink',
      CareOperation.clean =>
        itemId == null || itemId == 'soap' || itemId == 'shampoo',
      CareOperation.play || CareOperation.sleep => itemId == null,
    };
    if (!allowedItem) {
      throw ArgumentError.value(
        itemId,
        'itemId',
        'is not supported for ${operation.apiValue}',
      );
    }
    return {
      'operationId': operationId,
      'aggregateType': 'pet',
      'aggregateId': petId,
      'baseRevision': baseRevision,
      'operationType': operation.apiValue,
      'arguments': <String, dynamic>{'itemId': ?itemId},
      'clientWallTime': clientWallTime.toUtc().toIso8601String(),
      'clientMonotonicOffsetMs': clientMonotonicOffsetMs,
      'schemaVersion': 1,
    };
  }
}

enum CareCommandStatus {
  applied('APPLIED'),
  alreadyApplied('ALREADY_APPLIED'),
  rejectedPrecondition('REJECTED_PRECONDITION'),
  rejectedExpired('REJECTED_EXPIRED'),
  rejectedInvalid('REJECTED_INVALID'),
  retryable('RETRYABLE');

  const CareCommandStatus(this.apiValue);

  factory CareCommandStatus.fromJson(String value) {
    return CareCommandStatus.values.firstWhere(
      (status) => status.apiValue == value,
      orElse: () => throw FormatException('unsupported care status $value'),
    );
  }

  final String apiValue;

  bool get isApplied => switch (this) {
    CareCommandStatus.applied || CareCommandStatus.alreadyApplied => true,
    _ => false,
  };
}

class CarePetSnapshot {
  const CarePetSnapshot({
    required this.id,
    required this.needs,
    required this.revision,
    required this.isWeak,
    required this.needsUpdatedAt,
    this.needsZeroSince,
    this.sleepingUntil,
  });

  factory CarePetSnapshot.fromJson(JsonMap json) {
    return CarePetSnapshot(
      id: requiredString(json, 'id'),
      needs: PetNeeds.fromJson(requiredMap(json, 'needs')),
      revision: rangedInt(json, 'revision', min: 0),
      isWeak: requiredBool(json, 'isWeak'),
      needsUpdatedAt: requiredDateTime(json, 'needsUpdatedAt'),
      needsZeroSince: optionalDateTime(json, 'needsZeroSince'),
      sleepingUntil: optionalDateTime(json, 'sleepingUntil'),
    );
  }

  final String id;
  final PetNeeds needs;
  final int revision;
  final bool isWeak;
  final DateTime needsUpdatedAt;
  final DateTime? needsZeroSince;
  final DateTime? sleepingUntil;
}

class CareCommandResult {
  const CareCommandResult({
    required this.operationId,
    required this.status,
    required this.snapshot,
    this.errorCode,
  });

  factory CareCommandResult.fromJson(JsonMap json) {
    return CareCommandResult(
      operationId: requiredString(json, 'operationId'),
      status: CareCommandStatus.fromJson(requiredString(json, 'status')),
      errorCode: optionalString(json, 'errorCode'),
      snapshot: CarePetSnapshot.fromJson(requiredMap(json, 'snapshot')),
    );
  }

  final String operationId;
  final CareCommandStatus status;
  final String? errorCode;
  final CarePetSnapshot snapshot;
}

class CareSyncResult {
  const CareSyncResult({
    required this.results,
    required this.canonicalSnapshots,
    required this.newRevision,
    required this.serverTime,
  });

  factory CareSyncResult.fromJson(JsonMap json) {
    final results = requiredList(json, 'results')
        .map((value) => CareCommandResult.fromJson(asMap(value, 'results[]')))
        .toList(growable: false);
    final snapshots = requiredList(json, 'canonicalSnapshots')
        .map(
          (value) =>
              CarePetSnapshot.fromJson(asMap(value, 'canonicalSnapshots[]')),
        )
        .toList(growable: false);
    if (results.isEmpty || snapshots.isEmpty) {
      throw const FormatException(
        'care response must contain results and canonical snapshots',
      );
    }
    return CareSyncResult(
      results: results,
      canonicalSnapshots: snapshots,
      newRevision: rangedInt(json, 'newRevision', min: 0),
      serverTime: requiredDateTime(json, 'serverTime'),
    );
  }

  final List<CareCommandResult> results;
  final List<CarePetSnapshot> canonicalSnapshots;
  final int newRevision;
  final DateTime serverTime;

  CareCommandResult resultFor(String operationId) {
    final matches = results.where(
      (result) => result.operationId == operationId,
    );
    if (matches.length != 1) {
      throw const FormatException(
        'care response does not match the submitted operation',
      );
    }
    return matches.single;
  }
}
