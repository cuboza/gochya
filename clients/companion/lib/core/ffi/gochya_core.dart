import 'dart:ffi';

import 'package:ffi/ffi.dart';

import 'core_bindings.dart';
import 'core_types.dart';

export 'core_types.dart'
    show
        CoreCareAction,
        CoreCareItem,
        CoreException,
        CoreNeedsState,
        CoreStatus,
        coreAbiVersion;

/// Typed entry point to the Shared Core.
///
/// Every formula lives in the Core (`CORE_FORMULAS.md`); this class only
/// marshals values across the C ABI and turns a non-`Ok` status into an
/// exception. It deliberately exposes no arithmetic of its own.
///
/// The Core is *not* an authority here. The server stays authoritative for
/// battle, economy, genome and rewards; these calls exist so the phone can
/// predict care locally between server reads using the very same formulas
/// rather than a client-side approximation of them.
class GochyaCore {
  GochyaCore(this._bindings);

  /// Opens the platform library and refuses a Core built from another
  /// revision. [libraryPath] is for tests and tooling on host platforms.
  factory GochyaCore.open({String? libraryPath}) {
    final bindings = CoreBindings.open(libraryPath: libraryPath);
    bindings.assertAbiVersion();
    return GochyaCore(bindings);
  }

  final CoreBindings _bindings;

  /// ABI reported by the loaded library.
  int get abiVersion => _bindings.abiVersion();

  /// Decays [state] forward by [elapsed].
  ///
  /// Fractional decay is carried in the remainder fields, so advancing in many
  /// small steps lands on the same state as one long step.
  ///
  /// The Core refuses an [elapsed] longer than 24 hours with
  /// [CoreStatus.domainRejected], and that rejection is surfaced rather than
  /// clamped: after a longer absence the server's value is the only correct
  /// answer, and clamping would be the client inventing a decay curve.
  CoreNeedsState advanceNeeds(CoreNeedsState state, Duration elapsed) {
    if (elapsed.isNegative) {
      throw ArgumentError.value(elapsed, 'elapsed', 'must not be negative');
    }
    return _call(
      state,
      (input, output) =>
          _bindings.advanceNeeds(input, elapsed.inSeconds, output),
    );
  }

  /// Applies a care action to [state].
  ///
  /// This is a local prediction of what the server will confirm, never a
  /// substitute for it: the authoritative state still comes back from the care
  /// mutation, which is idempotent on retry.
  CoreNeedsState applyCare(
    CoreNeedsState state, {
    required CoreCareAction action,
    CoreCareItem item = CoreCareItem.none,
  }) {
    return _call(
      state,
      (input, output) =>
          _bindings.applyCare(input, action.code, item.code, output),
    );
  }

  CoreNeedsState _call(
    CoreNeedsState state,
    int Function(
      Pointer<GochyaNeedsStateV1> input,
      Pointer<GochyaNeedsStateV1> output,
    )
    invoke,
  ) {
    return using((arena) {
      final input = arena<GochyaNeedsStateV1>();
      final output = arena<GochyaNeedsStateV1>();
      state.writeTo(input);
      final status = CoreStatus.fromCode(invoke(input, output));
      if (status != CoreStatus.ok) {
        throw CoreException(status, 'Shared Core rejected the needs call');
      }
      return CoreNeedsState.fromStruct(output.ref);
    });
  }
}
