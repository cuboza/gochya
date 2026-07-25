import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/device/installation_id_store.dart';
import '../../core/identifiers/uuid_v4.dart';
import '../../core/models/care_models.dart';
import '../../core/network/api_providers.dart';
import '../../core/network/gochya_api_client.dart';
import '../session/session_request_runner.dart';
import 'care_queue_store.dart';

final careRepositoryProvider = Provider<CareRepository>(
  (ref) => ApiCareRepository(
    api: ref.watch(apiClientProvider),
    installationIdStore: ref.watch(installationIdStoreProvider),
    queueStore: ref.watch(careQueueStoreProvider),
    sessionRunner: ref.watch(sessionRequestRunnerProvider),
  ),
);

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
    required this.installationIdStore,
    required this.queueStore,
    required this.sessionRunner,
  });

  final GochyaApiClient api;
  final InstallationIdStore installationIdStore;
  final CareQueueStore queueStore;
  final AuthenticatedRequestRunner sessionRunner;

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
        final deviceId = await installationIdStore.getOrCreate();
        response = await sessionRunner.run(
          accessToken: accessToken,
          request: (token) => api.reconcileCareBatch(
            accessToken: token,
            deviceId: deviceId,
            commands: batch,
          ),
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
