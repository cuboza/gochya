import 'dart:ffi';

/// ABI the client is compiled against. `gochya_abi_version` must return exactly
/// this: a Core built from another revision computes different formulas, and a
/// silent mismatch would let the phone disagree with the server.
const int coreAbiVersion = 0x0002_0300;

/// Schema version stamped into every versioned struct header.
const int coreAbiSchemaVersion = 1;

/// Mirrors `GochyaStatus` in `core/ffi/gochya_core.h`.
enum CoreStatus {
  ok(0),
  invalidArgument(1),
  bufferTooSmall(2),
  schemaMismatch(3),
  domainRejected(4),
  internalError(255);

  const CoreStatus(this.code);

  final int code;

  static CoreStatus fromCode(int code) {
    for (final status in CoreStatus.values) {
      if (status.code == code) {
        return status;
      }
    }
    throw CoreException(
      CoreStatus.internalError,
      'core returned unknown status $code',
    );
  }
}

/// Raised when the Core rejects a call. Never swallowed into a default value:
/// a rejected call means the caller's input was outside the domain the Core
/// accepts, and guessing a replacement would invent a formula on the client.
class CoreException implements Exception {
  const CoreException(this.status, this.message);

  final CoreStatus status;
  final String message;

  @override
  String toString() => 'CoreException(${status.name}): $message';
}

/// Mirrors `CareAction` in `core/src/pet.rs`.
enum CoreCareAction {
  feed(0),
  clean(1),
  play(2),
  sleep(3);

  const CoreCareAction(this.code);

  final int code;
}

/// Mirrors `CareItem` in `core/src/pet.rs`.
enum CoreCareItem {
  none(0),
  apple(1),
  steak(2),
  energyDrink(3),
  soap(4),
  shampoo(5);

  const CoreCareItem(this.code);

  final int code;
}

/// Mirrors `GochyaNeedsStateV1`. Field order and types must stay identical to
/// the C header: Dart lays the struct out with the same natural alignment, and
/// `coreNeedsStateSize` is asserted against the header at test time.
final class GochyaNeedsStateV1 extends Struct {
  @Uint32()
  external int structSize;

  @Uint16()
  external int schemaVersion;

  @Uint8()
  external int isSleeping;

  @Uint8()
  external int isWeak;

  @Uint8()
  external int hunger;

  @Uint8()
  external int energy;

  @Uint8()
  external int hygiene;

  @Uint8()
  external int mood;

  @Uint32()
  external int hungerRemainder;

  @Uint32()
  external int energyRemainder;

  @Uint32()
  external int hygieneRemainder;

  @Uint32()
  external int moodRemainder;

  @Uint64()
  external int zeroStreakSeconds;

  @Array(16)
  external Array<Uint8> reserved;
}

/// Size the Core expects for [GochyaNeedsStateV1], taken from the C header.
const int coreNeedsStateSize = 56;

/// Dart-side view of a pet's needs. Construction is strict for the same reason
/// the JSON decoders are: an impossible value is a defect, not something to
/// clamp quietly and hand to the Core.
class CoreNeedsState {
  CoreNeedsState({
    required this.hunger,
    required this.energy,
    required this.hygiene,
    required this.mood,
    this.hungerRemainder = 0,
    this.energyRemainder = 0,
    this.hygieneRemainder = 0,
    this.moodRemainder = 0,
    this.zeroStreakSeconds = 0,
    this.isSleeping = false,
    this.isWeak = false,
  }) {
    _checkNeed('hunger', hunger);
    _checkNeed('energy', energy);
    _checkNeed('hygiene', hygiene);
    _checkNeed('mood', mood);
    _checkRemainder('hungerRemainder', hungerRemainder);
    _checkRemainder('energyRemainder', energyRemainder);
    _checkRemainder('hygieneRemainder', hygieneRemainder);
    _checkRemainder('moodRemainder', moodRemainder);
    if (zeroStreakSeconds < 0) {
      throw ArgumentError.value(
        zeroStreakSeconds,
        'zeroStreakSeconds',
        'must not be negative',
      );
    }
  }

  final int hunger;
  final int energy;
  final int hygiene;
  final int mood;

  /// Sub-point decay carried between calls. Opaque to the client — it exists so
  /// repeated short advances decay at the same rate as one long advance.
  final int hungerRemainder;
  final int energyRemainder;
  final int hygieneRemainder;
  final int moodRemainder;

  final int zeroStreakSeconds;
  final bool isSleeping;
  final bool isWeak;

  static void _checkNeed(String name, int value) {
    if (value < 0 || value > 100) {
      throw ArgumentError.value(value, name, 'must be within 0..100');
    }
  }

  static void _checkRemainder(String name, int value) {
    if (value < 0 || value > 0xFFFFFFFF) {
      throw ArgumentError.value(value, name, 'must fit an unsigned 32-bit int');
    }
  }

  /// Writes this state into Core-owned memory, stamping the versioned header.
  void writeTo(Pointer<GochyaNeedsStateV1> pointer) {
    final struct = pointer.ref;
    struct.structSize = coreNeedsStateSize;
    struct.schemaVersion = coreAbiSchemaVersion;
    struct.isSleeping = isSleeping ? 1 : 0;
    struct.isWeak = isWeak ? 1 : 0;
    struct.hunger = hunger;
    struct.energy = energy;
    struct.hygiene = hygiene;
    struct.mood = mood;
    struct.hungerRemainder = hungerRemainder;
    struct.energyRemainder = energyRemainder;
    struct.hygieneRemainder = hygieneRemainder;
    struct.moodRemainder = moodRemainder;
    struct.zeroStreakSeconds = zeroStreakSeconds;
    for (var index = 0; index < 16; index += 1) {
      struct.reserved[index] = 0;
    }
  }

  factory CoreNeedsState.fromStruct(GochyaNeedsStateV1 struct) {
    return CoreNeedsState(
      hunger: struct.hunger,
      energy: struct.energy,
      hygiene: struct.hygiene,
      mood: struct.mood,
      hungerRemainder: struct.hungerRemainder,
      energyRemainder: struct.energyRemainder,
      hygieneRemainder: struct.hygieneRemainder,
      moodRemainder: struct.moodRemainder,
      zeroStreakSeconds: struct.zeroStreakSeconds,
      isSleeping: struct.isSleeping != 0,
      isWeak: struct.isWeak != 0,
    );
  }

  @override
  bool operator ==(Object other) {
    return other is CoreNeedsState &&
        other.hunger == hunger &&
        other.energy == energy &&
        other.hygiene == hygiene &&
        other.mood == mood &&
        other.hungerRemainder == hungerRemainder &&
        other.energyRemainder == energyRemainder &&
        other.hygieneRemainder == hygieneRemainder &&
        other.moodRemainder == moodRemainder &&
        other.zeroStreakSeconds == zeroStreakSeconds &&
        other.isSleeping == isSleeping &&
        other.isWeak == isWeak;
  }

  @override
  int get hashCode => Object.hash(
    hunger,
    energy,
    hygiene,
    mood,
    hungerRemainder,
    energyRemainder,
    hygieneRemainder,
    moodRemainder,
    zeroStreakSeconds,
    isSleeping,
    isWeak,
  );

  @override
  String toString() {
    return 'CoreNeedsState(hunger: $hunger, energy: $energy, '
        'hygiene: $hygiene, mood: $mood, sleeping: $isSleeping, '
        'weak: $isWeak)';
  }
}
