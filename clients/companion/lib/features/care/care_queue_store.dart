import 'dart:convert';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import '../../core/models/care_models.dart';
import '../../core/models/profile_models.dart';

const maxPendingCareCommands = 100;

final careQueueStoreProvider = Provider<CareQueueStore>(
  (ref) => SecureCareQueueStore(),
);

class CareQueue {
  const CareQueue({
    required this.accountId,
    required this.nextSequence,
    required this.commands,
  });

  factory CareQueue.empty(String accountId) {
    _validateAccountId(accountId);
    return CareQueue(accountId: accountId, nextSequence: 1, commands: const []);
  }

  factory CareQueue.fromJson(JsonMap json) {
    if (rangedInt(json, 'storageVersion', min: 1, max: 1) != 1) {
      throw const FormatException('unsupported care queue storage version');
    }
    final accountId = requiredString(json, 'accountId');
    final nextSequence = rangedInt(json, 'nextSequence', min: 1);
    final commands = requiredList(json, 'commands')
        .map(
          (value) =>
              QueuedCareCommand.fromStorageJson(asMap(value, 'commands[]')),
        )
        .toList(growable: false);
    _validateCommands(commands, nextSequence: nextSequence);
    return CareQueue(
      accountId: accountId,
      nextSequence: nextSequence,
      commands: commands,
    );
  }

  final String accountId;
  final int nextSequence;
  final List<QueuedCareCommand> commands;

  JsonMap toJson() {
    _validateAccountId(accountId);
    _validateCommands(commands, nextSequence: nextSequence);
    return {
      'storageVersion': 1,
      'accountId': accountId,
      'nextSequence': nextSequence,
      'commands': commands
          .map((command) => command.toStorageJson())
          .toList(growable: false),
    };
  }

  CareQueue append({
    required String petId,
    required int baseRevision,
    required CareIntent intent,
  }) {
    if (commands.length >= maxPendingCareCommands) {
      throw const CareQueueFullException();
    }
    final command = QueuedCareCommand(
      sequence: nextSequence,
      petId: petId,
      baseRevision: baseRevision,
      intent: intent,
    );
    return CareQueue(
      accountId: accountId,
      nextSequence: nextSequence + 1,
      commands: [...commands, command],
    );
  }

  CareQueue removeOperationIds(Set<String> operationIds) {
    if (operationIds.isEmpty) {
      return this;
    }
    return CareQueue(
      accountId: accountId,
      nextSequence: nextSequence,
      commands: commands
          .where(
            (command) => !operationIds.contains(command.intent.operationId),
          )
          .toList(growable: false),
    );
  }

  int nextBaseRevision(String petId, int canonicalRevision) {
    QueuedCareCommand? last;
    for (final command in commands) {
      if (command.petId == petId) {
        last = command;
      }
    }
    return last == null ? canonicalRevision : last.baseRevision + 1;
  }
}

abstract interface class CareQueueStore {
  Future<CareQueue> loadForAccount(String accountId);

  Future<void> save(CareQueue queue);

  Future<void> clear();
}

abstract interface class CareQueueStorage {
  Future<String?> read();

  Future<void> write(String value);

  Future<void> clear();
}

class FlutterSecureCareQueueStorage implements CareQueueStorage {
  FlutterSecureCareQueueStorage({FlutterSecureStorage? storage})
    : _storage = storage ?? const FlutterSecureStorage();

  static const _queueKey = 'gochya.care_queue.v1';

  final FlutterSecureStorage _storage;

  @override
  Future<String?> read() => _storage.read(key: _queueKey);

  @override
  Future<void> write(String value) {
    return _storage.write(key: _queueKey, value: value);
  }

  @override
  Future<void> clear() => _storage.delete(key: _queueKey);
}

class SecureCareQueueStore implements CareQueueStore {
  SecureCareQueueStore({CareQueueStorage? storage})
    : _storage = storage ?? FlutterSecureCareQueueStorage();

  final CareQueueStorage _storage;

  @override
  Future<CareQueue> loadForAccount(String accountId) async {
    _validateAccountId(accountId);
    final encoded = await _storage.read();
    if (encoded == null) {
      return CareQueue.empty(accountId);
    }

    late final CareQueue queue;
    try {
      final decoded = jsonDecode(encoded);
      queue = CareQueue.fromJson(asMap(decoded, 'care queue'));
    } on FormatException catch (error) {
      throw CareQueueCorruptedException(error);
    } on ArgumentError catch (error) {
      throw CareQueueCorruptedException(error);
    }
    if (queue.accountId != accountId) {
      await _storage.clear();
      return CareQueue.empty(accountId);
    }
    return queue;
  }

  @override
  Future<void> save(CareQueue queue) {
    return _storage.write(jsonEncode(queue.toJson()));
  }

  @override
  Future<void> clear() => _storage.clear();
}

class CareQueueFullException implements Exception {
  const CareQueueFullException();
}

class CareQueueCorruptedException implements Exception {
  const CareQueueCorruptedException(this.cause);

  final Object cause;
}

void _validateAccountId(String accountId) {
  if (accountId.trim().isEmpty) {
    throw ArgumentError.value(accountId, 'accountId', 'must not be empty');
  }
}

void _validateCommands(
  List<QueuedCareCommand> commands, {
  required int nextSequence,
}) {
  if (commands.length > maxPendingCareCommands) {
    throw const FormatException('care queue exceeds its command limit');
  }
  var previousSequence = 0;
  final operationIds = <String>{};
  for (final command in commands) {
    if (command.sequence <= previousSequence ||
        command.sequence >= nextSequence) {
      throw const FormatException(
        'care queue sequence numbers are not strictly increasing',
      );
    }
    if (!operationIds.add(command.intent.operationId)) {
      throw const FormatException('care queue operationId must be unique');
    }
    command.toApiJson();
    previousSequence = command.sequence;
  }
}
