import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'gochya_core.dart';

/// Opens the Shared Core once per app run.
///
/// Returns null when the platform has no usable library — an iOS build without
/// the static link, or a debug run where the `.so` was never produced. Callers
/// fall back to the server's value, which is authoritative regardless, so a
/// missing Core degrades the prediction and nothing else.
///
/// The failure is logged rather than swallowed: a Core that silently never
/// loads looks identical to one that loads and agrees with the server, and
/// that is exactly the bug that would go unnoticed for months.
final gochyaCoreProvider = Provider<GochyaCore?>((ref) {
  try {
    final core = GochyaCore.open();
    debugPrint(
      'gochya: Shared Core loaded, ABI 0x${core.abiVersion.toRadixString(16)}',
    );
    return core;
  } on Object catch (error) {
    debugPrint('gochya: Shared Core unavailable — $error');
    return null;
  }
});
