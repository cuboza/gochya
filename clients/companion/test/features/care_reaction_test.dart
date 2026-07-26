import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gochya_companion/app/app.dart';
import 'package:gochya_companion/core/models/care_models.dart';
import 'package:gochya_companion/core/models/profile_models.dart';
import 'package:gochya_companion/core/session/session_store.dart';
import 'package:gochya_companion/features/care/care_queue_store.dart';
import 'package:gochya_companion/features/care/care_repository.dart';
import 'package:gochya_companion/features/creatures/creature_rig.dart';
import 'package:gochya_companion/features/creatures/rigged_creature.dart';
import 'package:gochya_companion/features/home/profile_repository.dart';
import 'package:gochya_companion/features/session/session_controller.dart';

void main() {
  // The suite runs reduced-motion by default, where a controller jumps
  // straight to its end. These tests are about the animation itself, so they
  // opt back into real motion and pump fixed durations instead of settling —
  // the idle loop never ends.
  setUp(() {
    TestWidgetsFlutterBinding
            .instance
            .platformDispatcher
            .accessibilityFeaturesTestValue =
        const FakeAccessibilityFeatures();
  });

  tearDown(() {
    TestWidgetsFlutterBinding
        .instance
        .platformDispatcher
        .accessibilityFeaturesTestValue = const FakeAccessibilityFeatures(
      disableAnimations: true,
    );
  });

  testWidgets('a confirmed feed makes the pet react', (tester) async {
    await _pumpHome(tester);

    expect(_action(tester), CreatureAction.idle);

    await tester.tap(find.byKey(const Key('care-feed')));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 200));

    expect(_action(tester), CreatureAction.eat);

    // The reaction is a one-shot: it returns to idle when it finishes.
    await tester.pump(const Duration(milliseconds: 700));
    expect(_action(tester), CreatureAction.idle);
  });

  testWidgets('a queued command leaves the pet idle', (tester) async {
    await _pumpHome(tester, queued: true);

    await tester.tap(find.byKey(const Key('care-feed')));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 200));

    // Nothing was confirmed by the server, so nothing is celebrated.
    expect(_action(tester), CreatureAction.idle);
  });

  testWidgets('a sleeping pet keeps its sleeping pose', (tester) async {
    await _pumpHome(tester, sleeping: true);

    expect(_action(tester), CreatureAction.sleeping);
  });

  testWidgets('reduce motion skips the reaction and its particles', (
    tester,
  ) async {
    // This test alone runs in the suite's default reduced-motion mode.
    TestWidgetsFlutterBinding
        .instance
        .platformDispatcher
        .accessibilityFeaturesTestValue = const FakeAccessibilityFeatures(
      disableAnimations: true,
    );
    await _pumpHome(tester);

    await tester.tap(find.byKey(const Key('care-feed')));

    // Stepping frame by frame matters here. Flutter already shortens the
    // controller to 5% under reduce motion, so a single late pump would find
    // the reaction finished and pass whether or not it ever ran. The reaction
    // must never start, so every intermediate frame is checked.
    // The flying treat is rendered only while the action is `eat`, so an
    // action that never leaves idle also means no particle was emitted.
    for (var frame = 0; frame < 12; frame += 1) {
      await tester.pump(const Duration(milliseconds: 5));
      expect(_action(tester), CreatureAction.idle);
    }
  });
}

CreatureAction _action(WidgetTester tester) {
  return tester.widget<RiggedCreature>(find.byType(RiggedCreature)).action;
}

Future<void> _pumpHome(
  WidgetTester tester, {
  bool queued = false,
  bool sleeping = false,
}) async {
  await tester.binding.setSurfaceSize(const Size(1000, 2400));
  addTearDown(() => tester.binding.setSurfaceSize(null));
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        sessionStoreProvider.overrideWithValue(_MemorySessionStore()),
        careQueueStoreProvider.overrideWithValue(_MemoryCareQueueStore()),
        profileRepositoryProvider.overrideWithValue(
          _FakeProfileRepository(sleeping: sleeping),
        ),
        careRepositoryProvider.overrideWithValue(
          _FakeCareRepository(queued: queued),
        ),
      ],
      child: const GochyaApp(),
    ),
  );
  // Fixed pumps: the creature idle loop would keep pumpAndSettle spinning.
  await tester.pump();
  await tester.pump(const Duration(milliseconds: 50));
}

class _FakeProfileRepository implements ProfileRepository {
  const _FakeProfileRepository({required this.sleeping});

  final bool sleeping;

  @override
  Future<HomeSnapshot> loadHome(String accessToken) async {
    return HomeSnapshot(
      profile: _profile,
      pets: [_pet(sleeping: sleeping)],
    );
  }

  @override
  Future<LineageTree> loadLineage(String accessToken, String petId) =>
      throw UnsupportedError('this test never opens lineage');
}

class _FakeCareRepository implements CareRepository {
  const _FakeCareRepository({required this.queued});

  final bool queued;

  @override
  Future<void> clearQueue() async {}

  @override
  Future<CareReconcileResult> reconcilePending({
    required String accountId,
    required String accessToken,
  }) async => const CareReconcileResult(results: [], pendingCount: 0);

  @override
  Future<CareSubmitResult> submit({
    required String accountId,
    required String accessToken,
    required String petId,
    required int canonicalRevision,
    required CareIntent intent,
  }) async {
    final snapshot = CarePetSnapshot(
      id: petId,
      needs: const PetNeeds(hunger: 93, energy: 72, hygiene: 65, mood: 94),
      revision: canonicalRevision + 1,
      isWeak: false,
      needsUpdatedAt: DateTime.now().toUtc(),
    );
    if (queued) {
      return const CareSubmitResult(
        commandResult: null,
        canonicalSnapshot: null,
        pendingCount: 1,
      );
    }
    return CareSubmitResult(
      commandResult: CareCommandResult(
        operationId: intent.operationId,
        status: CareCommandStatus.applied,
        snapshot: snapshot,
      ),
      canonicalSnapshot: snapshot,
      pendingCount: 0,
    );
  }
}

class _MemorySessionStore implements SessionStore {
  SessionTokens? _tokens = const SessionTokens(
    accessToken: 'access',
    refreshToken: 'refresh',
  );

  @override
  Future<void> clear() async => _tokens = null;

  @override
  Future<SessionTokens?> read() async => _tokens;

  @override
  Future<void> write(SessionTokens tokens) async => _tokens = tokens;
}

class _MemoryCareQueueStore implements CareQueueStore {
  CareQueue? _queue;

  @override
  Future<void> clear() async => _queue = null;

  @override
  Future<CareQueue> loadForAccount(String accountId) async =>
      _queue ?? CareQueue.empty(accountId);

  @override
  Future<void> save(CareQueue queue) async => _queue = queue;
}

final _profile = PlayerProfile(
  id: 'player-1',
  username: 'nika',
  displayName: 'Ника',
  createdAt: DateTime.parse('2026-07-20T10:00:00Z'),
  streakDays: 3,
  activePetId: 'pet-1',
);

PetSummary _pet({required bool sleeping}) {
  return PetSummary(
    id: 'pet-1',
    ownerId: 'player-1',
    genome: const {'element': 2},
    name: 'Моти',
    stage: 'baby',
    level: 4,
    xp: 320,
    needs: const PetNeeds(hunger: 81, energy: 72, hygiene: 65, mood: 94),
    stats: const PetStats(strength: 2, agility: 3, endurance: 4, focus: 5),
    generation: 1,
    isActive: true,
    createdAt: DateTime.parse('2026-07-20T10:00:00Z'),
    isWeak: false,
    careRevision: 9,
    needsUpdatedAt: DateTime.parse('2026-07-25T12:00:00Z'),
    sleepingUntil: sleeping
        ? DateTime.now().toUtc().add(const Duration(hours: 6))
        : null,
  );
}
