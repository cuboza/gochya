import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:gochya_companion/core/ffi/gochya_core.dart';
import 'package:gochya_companion/core/models/profile_models.dart';
import 'package:gochya_companion/features/home/needs_prediction.dart';

/// Runs against the real Shared Core — the prediction is only trustworthy if
/// the numbers come from the same code the server calls.
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

  final serverSpoke = DateTime.utc(2026, 7, 26, 12);

  PetSummary pet({
    PetNeeds needs = const PetNeeds(
      hunger: 80,
      energy: 70,
      hygiene: 60,
      mood: 50,
    ),
    DateTime? sleepingUntil,
    bool isWeak = false,
  }) {
    return PetSummary(
      id: 'pet-1',
      ownerId: 'player-1',
      genome: const {'element': 2},
      name: 'Моти',
      stage: 'baby',
      level: 4,
      xp: 320,
      needs: needs,
      stats: const PetStats(strength: 2, agility: 3, endurance: 4, focus: 5),
      generation: 1,
      isActive: true,
      createdAt: DateTime.utc(2026, 7, 20, 10),
      isWeak: isWeak,
      careRevision: 9,
      needsUpdatedAt: serverSpoke,
      sleepingUntil: sleepingUntil,
    );
  }

  test('decays the pet forward from the server timestamp', () {
    final predicted = predictNeeds(
      core: core,
      pet: pet(),
      now: serverSpoke.add(const Duration(hours: 6)),
    );

    expect(predicted.hunger, lessThan(80));
  });

  test('keeps the server value when no time has passed', () {
    final predicted = predictNeeds(core: core, pet: pet(), now: serverSpoke);

    expect(predicted.hunger, 80);
    expect(predicted.energy, 70);
  });

  test('keeps the server value when the clock runs backwards', () {
    // A device clock behind the server must not resurrect spent needs.
    final predicted = predictNeeds(
      core: core,
      pet: pet(),
      now: serverSpoke.subtract(const Duration(hours: 3)),
    );

    expect(predicted.hunger, 80);
  });

  test('falls back to the server value past the Core limit', () {
    // Beyond 24 hours the Core refuses, and the refusal must surface as "show
    // what the server said" rather than a clamped local guess.
    final predicted = predictNeeds(
      core: core,
      pet: pet(),
      now: serverSpoke.add(const Duration(days: 3)),
    );

    expect(predicted.hunger, 80);
    expect(predicted.energy, 70);
    expect(predicted.hygiene, 60);
    expect(predicted.mood, 50);
  });

  test('decays more slowly while the pet is asleep', () {
    final now = serverSpoke.add(const Duration(hours: 6));
    final awake = predictNeeds(core: core, pet: pet(), now: now);
    final asleep = predictNeeds(
      core: core,
      pet: pet(sleepingUntil: serverSpoke.add(const Duration(hours: 8))),
      now: now,
    );

    expect(asleep.hunger, greaterThan(awake.hunger));
  });

  test('treats a finished sleep as awake', () {
    final now = serverSpoke.add(const Duration(hours: 6));
    final awake = predictNeeds(core: core, pet: pet(), now: now);
    final wokeUp = predictNeeds(
      core: core,
      pet: pet(sleepingUntil: serverSpoke.add(const Duration(hours: 1))),
      now: now,
    );

    expect(wokeUp.hunger, awake.hunger);
  });

  test('shows the server value when the Core is unavailable', () {
    // An iOS build without the static link, or any platform with no library.
    final predicted = predictNeeds(
      core: null,
      pet: pet(),
      now: serverSpoke.add(const Duration(hours: 6)),
    );

    expect(predicted.hunger, 80);
  });
}
