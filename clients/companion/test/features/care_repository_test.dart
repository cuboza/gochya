import 'package:flutter_test/flutter_test.dart';
import 'package:gochya_companion/core/models/care_models.dart';
import 'package:gochya_companion/core/models/profile_models.dart';
import 'package:gochya_companion/core/network/gochya_api_client.dart';
import 'package:gochya_companion/features/care/care_queue_store.dart';
import 'package:gochya_companion/features/care/care_repository.dart';

void main() {
  test(
    'network uncertainty persists and retries the identical command',
    () async {
      final storage = _MemoryQueueStorage();
      final queueStore = SecureCareQueueStore(storage: storage);
      final api = _RecordingCareApi(failRequests: 1);
      final intent = _intent('10000000-0000-4000-8000-000000000001');
      final firstRepository = ApiCareRepository(
        api: api,
        deviceStore: const _DeviceStore(),
        queueStore: queueStore,
      );

      final submitted = await firstRepository.submit(
        accountId: 'player-1',
        accessToken: 'token',
        petId: _petId,
        canonicalRevision: 7,
        intent: intent,
      );

      expect(submitted.isQueued, isTrue);
      expect(submitted.pendingCount, 1);
      expect(storage.value, isNotNull);
      final persisted = await queueStore.loadForAccount('player-1');
      expect(persisted.commands, hasLength(1));
      _expectSameIntent(persisted.commands.single.intent, intent);

      final restartedRepository = ApiCareRepository(
        api: api,
        deviceStore: const _DeviceStore(),
        queueStore: queueStore,
      );
      final reconciled = await restartedRepository.reconcilePending(
        accountId: 'player-1',
        accessToken: 'token',
      );

      expect(reconciled.pendingCount, 0);
      expect(reconciled.results.single.status, CareCommandStatus.applied);
      expect(api.batches, hasLength(2));
      _expectSameCommand(api.batches.first.single, api.batches.last.single);
      expect((await queueStore.loadForAccount('player-1')).commands, isEmpty);
    },
  );

  test(
    'offline commands retain sequence and anticipated revisions in a batch',
    () async {
      final queueStore = SecureCareQueueStore(storage: _MemoryQueueStorage());
      final api = _RecordingCareApi(failRequests: 2);
      final repository = ApiCareRepository(
        api: api,
        deviceStore: const _DeviceStore(),
        queueStore: queueStore,
      );
      final first = _intent('10000000-0000-4000-8000-000000000001');
      final second = _intent(
        '10000000-0000-4000-8000-000000000002',
        operation: CareOperation.clean,
      );

      await repository.submit(
        accountId: 'player-1',
        accessToken: 'token',
        petId: _petId,
        canonicalRevision: 12,
        intent: first,
      );
      await repository.submit(
        accountId: 'player-1',
        accessToken: 'token',
        petId: _petId,
        canonicalRevision: 12,
        intent: second,
      );
      final queued = await queueStore.loadForAccount('player-1');
      expect(queued.commands.map((command) => command.sequence), [1, 2]);
      expect(queued.commands.map((command) => command.baseRevision), [12, 13]);

      final reconciled = await repository.reconcilePending(
        accountId: 'player-1',
        accessToken: 'token',
      );

      expect(reconciled.pendingCount, 0);
      expect(api.batches.last.map((command) => command.sequence), [1, 2]);
      expect(api.batches.last.map((command) => command.intent.operationId), [
        first.operationId,
        second.operationId,
      ]);
      expect(reconciled.canonicalSnapshot?.revision, 14);
    },
  );

  test(
    'terminal results are removed while retryable commands stay queued',
    () async {
      final queueStore = SecureCareQueueStore(storage: _MemoryQueueStorage());
      final api = _RecordingCareApi(
        failRequests: 2,
        nextStatuses: const [
          CareCommandStatus.applied,
          CareCommandStatus.retryable,
        ],
      );
      final repository = ApiCareRepository(
        api: api,
        deviceStore: const _DeviceStore(),
        queueStore: queueStore,
      );
      final first = _intent('10000000-0000-4000-8000-000000000001');
      final second = _intent(
        '10000000-0000-4000-8000-000000000002',
        operation: CareOperation.play,
      );

      await repository.submit(
        accountId: 'player-1',
        accessToken: 'token',
        petId: _petId,
        canonicalRevision: 2,
        intent: first,
      );
      await repository.submit(
        accountId: 'player-1',
        accessToken: 'token',
        petId: _petId,
        canonicalRevision: 2,
        intent: second,
      );
      final reconciled = await repository.reconcilePending(
        accountId: 'player-1',
        accessToken: 'token',
      );

      expect(reconciled.pendingCount, 1);
      final remaining = await queueStore.loadForAccount('player-1');
      expect(remaining.commands.single.intent.operationId, second.operationId);
    },
  );

  test('loading the journal for another account clears old commands', () async {
    final storage = _MemoryQueueStorage();
    final store = SecureCareQueueStore(storage: storage);
    final queued = CareQueue.empty('player-a').append(
      petId: _petId,
      baseRevision: 1,
      intent: _intent('10000000-0000-4000-8000-000000000001'),
    );
    await store.save(queued);

    final otherAccount = await store.loadForAccount('player-b');

    expect(otherAccount.accountId, 'player-b');
    expect(otherAccount.commands, isEmpty);
    expect(storage.wasCleared, isTrue);
  });

  test('corrupted journal fails closed without overwriting evidence', () async {
    final storage = _MemoryQueueStorage()..value = '{"storageVersion":1';
    final store = SecureCareQueueStore(storage: storage);

    await expectLater(
      store.loadForAccount('player-1'),
      throwsA(isA<CareQueueCorruptedException>()),
    );

    expect(storage.value, '{"storageVersion":1');
    expect(storage.wasCleared, isFalse);
  });

  test('journal enforces the one-hundred-command offline limit', () {
    var queue = CareQueue.empty('player-1');
    for (var index = 0; index < maxPendingCareCommands; index++) {
      queue = queue.append(
        petId: _petId,
        baseRevision: index,
        intent: _intent(
          '10000000-0000-4000-8000-${index.toString().padLeft(12, '0')}',
        ),
      );
    }

    expect(
      () => queue.append(
        petId: _petId,
        baseRevision: maxPendingCareCommands,
        intent: _intent('10000000-0000-4000-8000-999999999999'),
      ),
      throwsA(isA<CareQueueFullException>()),
    );
  });
}

const _petId = '20000000-0000-4000-8000-000000000001';

CareIntent _intent(
  String operationId, {
  CareOperation operation = CareOperation.feed,
}) {
  return CareIntent(
    operationId: operationId,
    operation: operation,
    itemId: operation == CareOperation.feed ? 'apple' : null,
    clientWallTime: DateTime.utc(2026, 7, 25, 12),
    clientMonotonicOffsetMs: 1234,
  );
}

void _expectSameCommand(QueuedCareCommand actual, QueuedCareCommand expected) {
  expect(actual.sequence, expected.sequence);
  expect(actual.petId, expected.petId);
  expect(actual.baseRevision, expected.baseRevision);
  _expectSameIntent(actual.intent, expected.intent);
}

void _expectSameIntent(CareIntent actual, CareIntent expected) {
  expect(actual.operationId, expected.operationId);
  expect(actual.operation, expected.operation);
  expect(actual.itemId, expected.itemId);
  expect(actual.clientWallTime, expected.clientWallTime);
  expect(actual.clientMonotonicOffsetMs, expected.clientMonotonicOffsetMs);
}

class _MemoryQueueStorage implements CareQueueStorage {
  String? value;
  bool wasCleared = false;

  @override
  Future<void> clear() async {
    value = null;
    wasCleared = true;
  }

  @override
  Future<String?> read() async => value;

  @override
  Future<void> write(String value) async {
    this.value = value;
  }
}

class _DeviceStore implements CareDeviceStore {
  const _DeviceStore();

  @override
  Future<String> getOrCreate() async {
    return '30000000-0000-4000-8000-000000000001';
  }
}

class _RecordingCareApi extends GochyaApiClient {
  _RecordingCareApi({required this.failRequests, this.nextStatuses})
    : super(baseUri: Uri.parse('https://api.example.com'));

  int failRequests;
  final List<CareCommandStatus>? nextStatuses;
  final List<List<QueuedCareCommand>> batches = [];

  @override
  Future<CareSyncResult> reconcileCareBatch({
    required String accessToken,
    required String deviceId,
    required List<QueuedCareCommand> commands,
  }) async {
    batches.add(List.unmodifiable(commands));
    if (failRequests > 0) {
      failRequests--;
      throw const ApiException(code: 'network_error', message: 'offline');
    }

    var revision = commands.first.baseRevision;
    final results = <CareCommandResult>[];
    for (var index = 0; index < commands.length; index++) {
      final command = commands[index];
      final status = nextStatuses?[index] ?? CareCommandStatus.applied;
      if (status == CareCommandStatus.applied) {
        revision++;
      }
      results.add(
        CareCommandResult(
          operationId: command.intent.operationId,
          status: status,
          snapshot: _snapshot(revision),
        ),
      );
    }
    return CareSyncResult(
      results: results,
      canonicalSnapshots: [_snapshot(revision)],
      newRevision: revision,
      serverTime: DateTime.utc(2026, 7, 25, 12, 1),
    );
  }
}

CarePetSnapshot _snapshot(int revision) {
  return CarePetSnapshot(
    id: _petId,
    needs: const PetNeeds(hunger: 80, energy: 70, hygiene: 60, mood: 90),
    revision: revision,
    isWeak: false,
    needsUpdatedAt: DateTime.utc(2026, 7, 25, 12, 1),
  );
}
