import 'dart:ffi';
import 'dart:io';

import 'core_types.dart';

typedef _AbiVersionNative = Uint32 Function();
typedef CoreAbiVersionFn = int Function();

typedef _AdvanceNeedsNative =
    Int32 Function(
      Pointer<GochyaNeedsStateV1> input,
      Uint64 elapsedSeconds,
      Pointer<GochyaNeedsStateV1> outState,
    );
typedef CoreAdvanceNeedsFn =
    int Function(
      Pointer<GochyaNeedsStateV1> input,
      int elapsedSeconds,
      Pointer<GochyaNeedsStateV1> outState,
    );

typedef _ApplyCareNative =
    Int32 Function(
      Pointer<GochyaNeedsStateV1> input,
      Uint8 action,
      Uint8 item,
      Pointer<GochyaNeedsStateV1> outState,
    );
typedef CoreApplyCareFn =
    int Function(
      Pointer<GochyaNeedsStateV1> input,
      int action,
      int item,
      Pointer<GochyaNeedsStateV1> outState,
    );

typedef _ApplyRestNative =
    Int32 Function(
      Pointer<GochyaNeedsStateV1> input,
      Uint16 sleepMinutes,
      Uint8 sleepQuality,
      Pointer<GochyaNeedsStateV1> outState,
    );
typedef CoreApplyRestFn =
    int Function(
      Pointer<GochyaNeedsStateV1> input,
      int sleepMinutes,
      int sleepQuality,
      Pointer<GochyaNeedsStateV1> outState,
    );

/// Opens the Shared Core for the current platform.
///
/// iOS links the Rust staticlib into the main binary, so symbols are looked up
/// with [DynamicLibrary.process]. `DynamicLibrary.open` on a bundled framework
/// is forbidden by App Review (`CLIENT_COMPANION.md` §6, audit T3): it survives
/// debug and fails in release, so it is not offered as a fallback here.
/// Android loads the per-ABI `.so` shipped in `jniLibs`. Host platforms are
/// only reachable from tests and tooling, which pass [libraryPath] explicitly.
DynamicLibrary openCoreLibrary({String? libraryPath}) {
  if (libraryPath != null) {
    return DynamicLibrary.open(libraryPath);
  }
  if (Platform.isIOS || Platform.isMacOS) {
    return DynamicLibrary.process();
  }
  if (Platform.isAndroid) {
    return DynamicLibrary.open('libgochya_core.so');
  }
  throw UnsupportedError(
    'Shared Core has no bundled library on ${Platform.operatingSystem}; '
    'build it with tools/build-core-host.sh and pass libraryPath explicitly',
  );
}

/// Raw symbol lookups. Callers should prefer the typed API in
/// `gochya_core.dart`; this layer exists so the marshalling stays testable
/// against a real library.
class CoreBindings {
  CoreBindings(DynamicLibrary library)
    : abiVersion = library.lookupFunction<_AbiVersionNative, CoreAbiVersionFn>(
        'gochya_abi_version',
      ),
      advanceNeeds = library
          .lookupFunction<_AdvanceNeedsNative, CoreAdvanceNeedsFn>(
            'gochya_advance_needs_v1',
          ),
      applyCare = library.lookupFunction<_ApplyCareNative, CoreApplyCareFn>(
        'gochya_apply_care_v1',
      ),
      applyRest = library.lookupFunction<_ApplyRestNative, CoreApplyRestFn>(
        'gochya_apply_rest_v1',
      );

  factory CoreBindings.open({String? libraryPath}) {
    return CoreBindings(openCoreLibrary(libraryPath: libraryPath));
  }

  final CoreAbiVersionFn abiVersion;
  final CoreAdvanceNeedsFn advanceNeeds;
  final CoreApplyCareFn applyCare;
  final CoreApplyRestFn applyRest;

  /// Fails closed when the loaded library cannot serve this client.
  ///
  /// Same rule as the server (`server/internal/corebridge/types.go`): the top
  /// half must match and the rest must not be older. A Core from a different
  /// major line computes different formulas than the server, which is worse
  /// than refusing to start; a newer minor is additive and fine.
  void assertAbiVersion() {
    final loaded = abiVersion();
    final sameLine = loaded >> 16 == coreMinimumAbiVersion >> 16;
    if (!sameLine || loaded < coreMinimumAbiVersion) {
      throw CoreException(
        CoreStatus.schemaMismatch,
        'Shared Core ABI 0x${loaded.toRadixString(16)} cannot serve this '
        'client, which needs 0x${coreMinimumAbiVersion.toRadixString(16)} '
        'through 0x0002ffff',
      );
    }
  }
}
