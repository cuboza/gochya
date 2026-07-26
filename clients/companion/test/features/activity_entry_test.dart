import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gochya_companion/app/app.dart';
import 'package:gochya_companion/core/models/activity_models.dart';
import 'package:gochya_companion/core/models/care_models.dart';
import 'package:gochya_companion/core/models/profile_models.dart';
import 'package:gochya_companion/core/session/session_store.dart';
import 'package:gochya_companion/features/activity/activity_repository.dart';
import 'package:gochya_companion/features/activity/activity_screen.dart';
import 'package:gochya_companion/features/care/care_queue_store.dart';
import 'package:gochya_companion/features/care/care_repository.dart';
import 'package:gochya_companion/features/home/profile_repository.dart';
import 'package:gochya_companion/features/session/session_controller.dart';

/// Symbiosis is a headline mechanic, so reaching it must not depend on the
/// player scrolling to the bottom of the home screen and noticing a card.
void main() {
  testWidgets('the activity entry is on screen without scrolling', (
    tester,
  ) async {
    await _pumpHome(tester);

    final action = find.byTooltip('Активность и Vitality');
    expect(action, findsOneWidget);
    // hitTestable proves it is actually reachable, not merely in the tree
    // behind something else or off screen.
    expect(action.hitTestable(), findsOneWidget);
  });

  testWidgets('the app bar entry opens the activity screen', (tester) async {
    await _pumpHome(tester);

    await tester.tap(find.byTooltip('Активность и Vitality'));
    await tester.pumpAndSettle();

    expect(find.byType(ActivityScreen), findsOneWidget);
    expect(find.textContaining('Vitality 118'), findsWidgets);
  });

  testWidgets('the symbiosis card sits above the lineage card', (tester) async {
    await _pumpHome(tester);

    final symbiosis = find.text('Симбиоз');
    final lineage = find.text('История рода');
    expect(symbiosis, findsOneWidget);
    expect(lineage, findsOneWidget);
    expect(
      tester.getTopLeft(symbiosis).dy,
      lessThan(tester.getTopLeft(lineage).dy),
    );
  });
}

Future<void> _pumpHome(WidgetTester tester) async {
  await tester.binding.setSurfaceSize(const Size(1000, 2400));
  addTearDown(() => tester.binding.setSurfaceSize(null));
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        sessionStoreProvider.overrideWithValue(_MemorySessionStore()),
        careQueueStoreProvider.overrideWithValue(_MemoryCareQueueStore()),
        profileRepositoryProvider.overrideWithValue(
          const _FakeProfileRepository(),
        ),
        careRepositoryProvider.overrideWithValue(const _FakeCareRepository()),
        activityRepositoryProvider.overrideWithValue(
          const _FakeActivityRepository(),
        ),
      ],
      child: const GochyaApp(),
    ),
  );
  await tester.pump();
  await tester.pump(const Duration(milliseconds: 50));
}

class _FakeProfileRepository implements ProfileRepository {
  const _FakeProfileRepository();

  @override
  Future<HomeSnapshot> loadHome(String accessToken) async {
    return HomeSnapshot(profile: _profile, pets: [_pet]);
  }

  @override
  Future<LineageTree> loadLineage(String accessToken, String petId) =>
      throw UnsupportedError('this test never opens lineage');
}

class _FakeActivityRepository implements ActivityRepository {
  const _FakeActivityRepository();

  @override
  Future<List<DailyActivity>> week(String accessToken) async {
    return [
      DailyActivity(
        date: '2026-07-25',
        snapshot: const ActivitySnapshotSummary(
          steps: 11240,
          sleepMinutes: 431,
          activeCalories: 380,
          workouts: 1,
        ),
        vitality: 118,
        vitalityAwarded: 118,
        statGains: const ActivityStatGains(
          strength: 1,
          agility: 2,
          endurance: 1,
          focus: 1,
        ),
        goals: const ActivityGoals(
          steps: 9000,
          sleepHours: 7.5,
          activeCalories: 420,
        ),
        sourceMetadata: 'health_connect://phone',
        updatedAt: DateTime.parse('2026-07-25T20:00:00Z'),
      ),
    ];
  }

  @override
  Future<ActivityRewardResult> claimReward(String accessToken) =>
      throw UnsupportedError('this test never claims');
}

class _FakeCareRepository implements CareRepository {
  const _FakeCareRepository();

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
  }) => throw UnsupportedError('this test never submits care');
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

final _pet = PetSummary(
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
);
