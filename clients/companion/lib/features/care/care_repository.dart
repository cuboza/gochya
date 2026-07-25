import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import '../../core/identifiers/uuid_v4.dart';
import '../../core/models/care_models.dart';
import '../../core/network/gochya_api_client.dart';
import '../home/profile_repository.dart';
import 'care_queue_store.dart';

final careDeviceStoreProvider = Provider<CareDeviceStore>(
  (ref) => SecureCareDeviceStore(),
);

final careQueueStoreProvider = Provider<CareQueueStore>(
  (ref) => SecureCareQueueStore(),
);

final careRepositoryProvider = Provider<CareRepository>(
  (ref) => ApiCareRepository(
    api: ref.watch(apiClientProvider),
    deviceStore: ref.watch(careDeviceStoreProvider),
    queueStore: ref.watch(careQueueStoreProvider),
  ),
);

abstract interface class CareDeviceStore {
  Future<String> getOrCreate();
}

class SecureCareDeviceStore implements CareDeviceStore {
  SecureCareDeviceStore({FlutterSecureStorage? storage})
    : _storage = storage ?? const FlutterSecureStorage();

  static const _deviceIdKey = 'gochya.care_device_id.v1';
  static final _uuidV4 = RegExp(
    r'^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-'
    r'[89ab][0-9a-f]{3}-[0-9a-f]{12}$',
  );

  final FlutterSecureStorage _storage;
  Future<String>? _deviceId;

  @override
  Future<String> getOrCreate() {
    return _deviceId ??= _loadOrCreate().catchError((Object error) {
      _deviceId = null;
      throw error;
    });
  }

  Future<String> _loadOrCreate() async {
    final stored = await _storage.read(key: _deviceIdKey);
    if (stored != null && _uuidV4.hasMatch(stored)) {
      return stored;
    }
    final created = newUuidV4();
    await _storage.write(key: _deviceIdKey, value: created);
    return created;
  }
}

abstract interface class CareRepository {
  Future<CareSubmitResult> submit({
    required String accountId,
    required String accessToken,
    required String petId,
    required int canonicalRevision,
    required CareIntent intent,
  });

  Future<CareReconcileResult> reconcilePending({
    required String accountId,
    required String accessToken,
  });

  Future<void> clearQueue();
}

class CareSubmitResult {
  const CareSubmitResult({
    required this.commandResult,
    required this.canonicalSnapshot,
    required this.pendingCount,
    this.syncError,
  });

  final CareCommandResult? commandResult;
  final CarePetSnapshot? canonicalSnapshot;
  final int pendingCount;
  final Object? syncError;

  bool get isQueued =>
      commandResult == null || !commandResult!.status.isTerminal;
}

class CareReconcileResult {
  const CareReconcileResult({
    required this.results,
    required this.pendingCount,
    this.canonicalSnapshot,
    this.syncError,
  });

  final List<CareCommandResult> results;
  final CarePetSnapshot? canonicalSnapshot;
  final int pendingCount;
  final Object? syncError;

  bool get changedCanonicalState =>
      results.any((result) => result.status.isTerminal);
}

class ApiCareRepository implements CareRepository {
  ApiCareRepository({
    required this.api,
    required this.deviceStore,
    required this.queueStore,
  });

  final GochyaApiClient api;
  final CareDeviceStore deviceStore;
  final CareQueueStore queueStore;

  Future<void> _tail = Future.value();

  @override
  Future<CareSubmitResult> submit({
    required String accountId,
    required String accessToken,
    required String petId,
    required int canonicalRevision,
    required CareIntent intent,
  }) {
    return _serialized(() async {
      var queue = await queueStore.loadForAccount(accountId);
      final baseRevision = queue.nextBaseRevision(petId, canonicalRevision);
      queue = queue.append(
        petId: petId,
        baseRevision: baseRevision,
        intent: intent,
      );
      await queueStore.save(queue);

      final reconciled = await _drain(accessToken: accessToken, queue: queue);
      CareCommandResult? commandResult;
      for (final result in reconciled.results) {
        if (result.operationId == intent.operationId) {
          commandResult = result;
          break;
        }
      }
      return CareSubmitResult(
        commandResult: commandResult,
        canonicalSnapshot: reconciled.canonicalSnapshot,
        pendingCount: reconciled.pendingCount,
        syncError: reconciled.syncError,
      );
    });
  }

  @override
  Future<CareReconcileResult> reconcilePending({
    required String accountId,
    required String accessToken,
  }) {
    return _serialized(() async {
      final queue = await queueStore.loadForAccount(accountId);
      return _drain(accessToken: accessToken, queue: queue);
    });
  }

  @override
  Future<void> clearQueue() {
    return _serialized(queueStore.clear);
  }

  Future<CareReconcileResult> _drain({
    required String accessToken,
    required CareQueue queue,
  }) async {
    final results = <CareCommandResult>[];
    CarePetSnapshot? canonicalSnapshot;
    var current = queue;
    while (current.commands.isNotEmpty) {
      final petId = current.commands.first.petId;
      final batch = current.commands
          .takeWhile((command) => command.petId == petId)
          .toList(growable: false);

      late final CareSyncResult response;
      try {
        response = await api.reconcileCareBatch(
          accessToken: accessToken,
          deviceId: await deviceStore.getOrCreate(),
          commands: batch,
        );
        _validateResponse(response, batch);
      } on ApiException catch (error) {
        if (error.isUnauthorized) {
          rethrow;
        }
        return CareReconcileResult(
          results: List.unmodifiable(results),
          canonicalSnapshot: canonicalSnapshot,
          pendingCount: current.commands.length,
          syncError: error,
        );
      }

      results.addAll(response.results);
      canonicalSnapshot = response.canonicalSnapshots.single;
      final confirmedIds = response.results
          .where((result) => result.status.isTerminal)
          .map((result) => result.operationId)
          .toSet();
      final updated = current.removeOperationIds(confirmedIds);
      if (updated.commands.length != current.commands.length) {
        await queueStore.save(updated);
      }
      current = updated;
      if (response.results.any(
        (result) => result.status == CareCommandStatus.retryable,
      )) {
        break;
      }
      if (confirmedIds.isEmpty) {
        break;
      }
    }
    return CareReconcileResult(
      results: List.unmodifiable(results),
      canonicalSnapshot: canonicalSnapshot,
      pendingCount: current.commands.length,
    );
  }

  void _validateResponse(
    CareSyncResult response,
    List<QueuedCareCommand> batch,
  ) {
    try {
      if (response.results.length != batch.length ||
          response.canonicalSnapshots.length != 1) {
        throw const FormatException(
          'care batch response has unexpected cardinality',
        );
      }
      final expectedIds = batch
          .map((command) => command.intent.operationId)
          .toSet();
      final actualIds = response.results
          .map((result) => result.operationId)
          .toSet();
      if (expectedIds.length != batch.length ||
          actualIds.length != response.results.length ||
          !expectedIds.containsAll(actualIds)) {
        throw const FormatException(
          'care batch response does not match submitted operations',
        );
      }
      final petId = batch.first.petId;
      final canonical = response.canonicalSnapshots.single;
      if (canonical.id != petId ||
          canonical.revision != response.newRevision ||
          response.results.any((result) => result.snapshot.id != petId)) {
        throw const FormatException(
          'care batch snapshots do not match the request',
        );
      }
    } on FormatException {
      throw const ApiException(
        code: 'invalid_response',
        message: 'Server returned an invalid care sync payload.',
      );
    }
  }

  Future<T> _serialized<T>(Future<T> Function() action) {
    final previous = _tail;
    final completer = Completer<T>();
    _tail = () async {
      try {
        await previous;
      } on Object {
        // A failed caller must not permanently poison the queue lock.
      }
      try {
        completer.complete(await action());
      } on Object catch (error, stackTrace) {
        completer.completeError(error, stackTrace);
      }
    }();
    return completer.future;
  }
}

final _processMonotonicClock = Stopwatch()..start();

CareIntent newCareIntent(CareOperation operation, {String? itemId}) {
  return CareIntent(
    operationId: newUuidV4(),
    operation: operation,
    itemId: itemId,
    clientWallTime: DateTime.now().toUtc(),
    clientMonotonicOffsetMs: _processMonotonicClock.elapsedMilliseconds,
  );
}
