import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import '../../core/identifiers/uuid_v4.dart';
import '../../core/models/care_models.dart';
import '../../core/network/gochya_api_client.dart';
import '../home/profile_repository.dart';

final careDeviceStoreProvider = Provider<CareDeviceStore>(
  (ref) => SecureCareDeviceStore(),
);

final careRepositoryProvider = Provider<CareRepository>(
  (ref) => ApiCareRepository(
    api: ref.watch(apiClientProvider),
    deviceStore: ref.watch(careDeviceStoreProvider),
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
  Future<CareCommandResult> execute({
    required String accessToken,
    required String petId,
    required int baseRevision,
    required CareIntent intent,
  });
}

class ApiCareRepository implements CareRepository {
  const ApiCareRepository({required this.api, required this.deviceStore});

  final GochyaApiClient api;
  final CareDeviceStore deviceStore;

  @override
  Future<CareCommandResult> execute({
    required String accessToken,
    required String petId,
    required int baseRevision,
    required CareIntent intent,
  }) async {
    final response = await api.reconcileCare(
      accessToken: accessToken,
      deviceId: await deviceStore.getOrCreate(),
      petId: petId,
      baseRevision: baseRevision,
      intent: intent,
    );
    try {
      final result = response.resultFor(intent.operationId);
      if (response.results.length != 1 ||
          response.canonicalSnapshots.length != 1) {
        throw const FormatException(
          'single-command care response has unexpected cardinality',
        );
      }
      final canonical = response.canonicalSnapshots.singleWhere(
        (snapshot) => snapshot.id == petId,
      );
      if (result.snapshot.id != petId ||
          result.snapshot.revision != canonical.revision ||
          canonical.revision != response.newRevision) {
        throw const FormatException('care snapshot does not match the request');
      }
      return result;
    } on StateError {
      throw const ApiException(
        code: 'invalid_response',
        message: 'Server returned an invalid care sync payload.',
      );
    } on FormatException {
      throw const ApiException(
        code: 'invalid_response',
        message: 'Server returned an invalid care sync payload.',
      );
    }
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
