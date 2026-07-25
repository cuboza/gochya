import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import '../identifiers/uuid_v4.dart';

final installationIdStoreProvider = Provider<InstallationIdStore>(
  (ref) => SecureInstallationIdStore(),
);

abstract interface class InstallationIdStore {
  Future<String> getOrCreate();
}

class SecureInstallationIdStore implements InstallationIdStore {
  SecureInstallationIdStore({FlutterSecureStorage? storage})
    : _storage = storage ?? const FlutterSecureStorage();

  // Keep the V1 key stable for installations created by the first care slice.
  static const _installationIdKey = 'gochya.care_device_id.v1';
  static final _uuidV4 = RegExp(
    r'^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-'
    r'[89ab][0-9a-f]{3}-[0-9a-f]{12}$',
  );

  final FlutterSecureStorage _storage;
  Future<String>? _installationId;

  @override
  Future<String> getOrCreate() {
    return _installationId ??= _loadOrCreate().catchError((Object error) {
      _installationId = null;
      throw error;
    });
  }

  Future<String> _loadOrCreate() async {
    final stored = await _storage.read(key: _installationIdKey);
    if (stored != null && _uuidV4.hasMatch(stored)) {
      return stored;
    }
    final created = newUuidV4();
    await _storage.write(key: _installationIdKey, value: created);
    return created;
  }
}
