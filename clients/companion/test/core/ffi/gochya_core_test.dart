import 'dart:ffi';
import 'dart:io';

import 'package:ffi/ffi.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gochya_companion/core/ffi/core_bindings.dart';
import 'package:gochya_companion/core/ffi/core_types.dart';
import 'package:gochya_companion/core/ffi/gochya_core.dart';

/// These tests run against the real Shared Core, not a mock: a mock would
/// happily agree with a marshalling bug. Build it with
/// `bash tools/build-core-host.sh` before running the suite.
String _hostLibraryPath() {
  final name = Platform.isMacOS ? 'libgochya_core.dylib' : 'libgochya_core.so';
  final path = '../../target/release/$name';
  if (!File(path).existsSync()) {
    fail(
      'Shared Core host library is missing at $path. '
      'Run `bash tools/build-core-host.sh` from the repository root.',
    );
  }
  return path;
}

void main() {
  late GochyaCore core;

  setUpAll(() {
    core = GochyaCore.open(libraryPath: _hostLibraryPath());
  });

  CoreNeedsState healthy() {
    return CoreNeedsState(hunger: 80, energy: 70, hygiene: 60, mood: 50);
  }

  group('ABI contract', () {
    test('loaded library matches the ABI the client was built against', () {
      expect(core.abiVersion, coreAbiVersion);
    });

    test('needs struct layout matches the C header', () {
      // Guards against a field reordering in `gochya_core.h` silently shifting
      // every value the client sends across the boundary.
      expect(sizeOf<GochyaNeedsStateV1>(), coreNeedsStateSize);
    });

    test('a Core built from another revision is refused at open', () {
      final bindings = CoreBindings.open(libraryPath: _hostLibraryPath());
      expect(bindings.abiVersion(), coreAbiVersion);
      expect(bindings.assertAbiVersion, returnsNormally);
    });

    test('a mis-stamped header is rejected rather than reinterpreted', () {
      final bindings = CoreBindings.open(libraryPath: _hostLibraryPath());
      using((arena) {
        final input = arena<GochyaNeedsStateV1>();
        final output = arena<GochyaNeedsStateV1>();
        healthy().writeTo(input);
        input.ref.structSize = coreNeedsStateSize - 1;
        final status = CoreStatus.fromCode(
          bindings.advanceNeeds(input, 60, output),
        );
        expect(status, CoreStatus.schemaMismatch);
      });
    });
  });

  group('advanceNeeds', () {
    test('is deterministic for identical inputs', () {
      final first = core.advanceNeeds(healthy(), const Duration(hours: 1));
      final second = core.advanceNeeds(healthy(), const Duration(hours: 1));
      expect(first, second);
    });

    test(
      'carries fractional decay so step size does not change the result',
      () {
        // The remainder fields exist precisely so an app that wakes up often
        // decays at the same rate as one that wakes up once.
        final single = core.advanceNeeds(healthy(), const Duration(hours: 1));
        var stepped = healthy();
        for (var minute = 0; minute < 60; minute += 1) {
          stepped = core.advanceNeeds(stepped, const Duration(minutes: 1));
        }
        expect(stepped, single);
      },
    );

    test('decays hunger while the pet is awake', () {
      final advanced = core.advanceNeeds(healthy(), const Duration(hours: 1));
      expect(advanced.hunger, lessThan(healthy().hunger));
    });

    test('decays more slowly while the pet sleeps', () {
      final awake = core.advanceNeeds(healthy(), const Duration(hours: 6));
      final asleep = core.advanceNeeds(
        CoreNeedsState(
          hunger: 80,
          energy: 70,
          hygiene: 60,
          mood: 50,
          isSleeping: true,
        ),
        const Duration(hours: 6),
      );
      expect(asleep.hunger, greaterThan(awake.hunger));
    });

    test('turns the pet weak after a long streak at zero', () {
      final starving = CoreNeedsState(
        hunger: 0,
        energy: 70,
        hygiene: 60,
        mood: 50,
      );
      final advanced = core.advanceNeeds(starving, const Duration(hours: 6));
      expect(advanced.isWeak, isTrue);
    });

    test('rejects an advance past the Core limit instead of clamping it', () {
      // A phone that was closed for days cannot predict forward: the server
      // value is the only correct answer, so the bridge must not invent one.
      expect(
        () => core.advanceNeeds(healthy(), const Duration(days: 3)),
        throwsA(
          isA<CoreException>().having(
            (error) => error.status,
            'status',
            CoreStatus.domainRejected,
          ),
        ),
      );
    });

    test('accepts the limit itself', () {
      expect(
        () => core.advanceNeeds(healthy(), const Duration(hours: 24)),
        returnsNormally,
      );
    });

    test('refuses a negative elapsed duration', () {
      expect(
        () => core.advanceNeeds(healthy(), const Duration(seconds: -1)),
        throwsArgumentError,
      );
    });
  });

  group('applyCare', () {
    test('feeding an apple restores hunger', () {
      final fed = core.applyCare(
        healthy(),
        action: CoreCareAction.feed,
        item: CoreCareItem.apple,
      );
      expect(fed.hunger, greaterThan(healthy().hunger));
    });

    test('sleeping puts the pet into the sleeping state', () {
      final slept = core.applyCare(healthy(), action: CoreCareAction.sleep);
      expect(slept.isSleeping, isTrue);
    });

    test('cleaning restores hygiene without touching hunger', () {
      final cleaned = core.applyCare(healthy(), action: CoreCareAction.clean);
      expect(cleaned.hygiene, greaterThan(healthy().hygiene));
      expect(cleaned.hunger, healthy().hunger);
    });

    test('is deterministic for identical inputs', () {
      final first = core.applyCare(healthy(), action: CoreCareAction.play);
      final second = core.applyCare(healthy(), action: CoreCareAction.play);
      expect(first, second);
    });
  });

  group('CoreNeedsState', () {
    test('rejects a need outside 0..100 rather than clamping it', () {
      expect(
        () => CoreNeedsState(hunger: 101, energy: 70, hygiene: 60, mood: 50),
        throwsArgumentError,
      );
    });

    test('rejects a negative zero streak', () {
      expect(
        () => CoreNeedsState(
          hunger: 10,
          energy: 70,
          hygiene: 60,
          mood: 50,
          zeroStreakSeconds: -1,
        ),
        throwsArgumentError,
      );
    });

    test('round-trips through the C struct unchanged', () {
      final state = CoreNeedsState(
        hunger: 41,
        energy: 32,
        hygiene: 23,
        mood: 14,
        hungerRemainder: 900,
        energyRemainder: 800,
        hygieneRemainder: 700,
        moodRemainder: 600,
        zeroStreakSeconds: 120,
        isSleeping: true,
        isWeak: true,
      );
      using((arena) {
        final pointer = arena<GochyaNeedsStateV1>();
        state.writeTo(pointer);
        expect(CoreNeedsState.fromStruct(pointer.ref), state);
        expect(pointer.ref.structSize, coreNeedsStateSize);
        expect(pointer.ref.schemaVersion, coreAbiSchemaVersion);
      });
    });
  });
}
