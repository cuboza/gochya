import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gochya_companion/app/theme.dart';
import 'package:gochya_companion/core/models/activity_models.dart';
import 'package:gochya_companion/core/models/technique_models.dart';
import 'package:gochya_companion/core/network/gochya_api_client.dart';
import 'package:gochya_companion/features/activity/activity_repository.dart';
import 'package:gochya_companion/features/activity/activity_screen.dart';

void main() {
  testWidgets('claims the daily card once the server unlocks it', (
    tester,
  ) async {
    final repository = _FakeActivityRepository();
    await _pumpActivity(tester, repository);

    expect(find.textContaining('Vitality 118'), findsWidgets);
    expect(find.textContaining('11240 шагов'), findsWidgets);

    await _tap(tester, find.text('Забрать карту дня'));
    await tester.pumpAndSettle();

    expect(repository.claimCalls, 1);
    expect(find.textContaining('Карта дня: Хук · Земля'), findsOneWidget);
    expect(
      tester
          .widget<FilledButton>(
            find.widgetWithText(FilledButton, 'Забрать карту дня'),
          )
          .onPressed,
      isNull,
    );
  });

  testWidgets('keeps the claim locked below the daily threshold', (
    tester,
  ) async {
    final repository = _FakeActivityRepository(vitality: 61);
    await _pumpActivity(tester, repository);

    expect(
      tester
          .widget<FilledButton>(
            find.widgetWithText(FilledButton, 'Забрать карту дня'),
          )
          .onPressed,
      isNull,
    );
    expect(repository.claimCalls, 0);
  });

  testWidgets('explains a server-side lock without inventing a reward', (
    tester,
  ) async {
    final repository = _FakeActivityRepository(
      claimError: const ApiException(
        statusCode: 409,
        code: 'activity_reward_locked',
        message: 'reward is locked',
      ),
    );
    await _pumpActivity(tester, repository);

    await _tap(tester, find.text('Забрать карту дня'));
    await tester.pumpAndSettle();

    expect(find.text('Нужно 100 Vitality за день.'), findsOneWidget);
    expect(find.textContaining('Карта дня'), findsNothing);
  });
}

Future<void> _tap(WidgetTester tester, Finder finder) async {
  await tester.ensureVisible(finder);
  await tester.pumpAndSettle();
  await tester.tap(finder);
  await tester.pump();
}

Future<void> _pumpActivity(
  WidgetTester tester,
  ActivityRepository repository,
) async {
  await tester.binding.setSurfaceSize(const Size(1000, 3000));
  addTearDown(() => tester.binding.setSurfaceSize(null));
  await tester.pumpWidget(
    ProviderScope(
      overrides: [activityRepositoryProvider.overrideWithValue(repository)],
      child: MaterialApp(
        theme: buildGochyaTheme(),
        home: const ActivityScreen(accessToken: 'access-token'),
      ),
    ),
  );
  await tester.pumpAndSettle();
}

class _FakeActivityRepository implements ActivityRepository {
  _FakeActivityRepository({this.vitality = 118, this.claimError});

  final int vitality;
  final Object? claimError;
  var claimCalls = 0;

  @override
  Future<List<DailyActivity>> week(String accessToken) async {
    expect(accessToken, 'access-token');
    return [_day(vitality)];
  }

  @override
  Future<ActivityRewardResult> claimReward(String accessToken) async {
    claimCalls++;
    if (claimError case final error?) {
      throw error;
    }
    return ActivityRewardResult(date: '2026-07-25', card: _card, awarded: true);
  }
}

DailyActivity _day(int vitality) {
  return DailyActivity(
    date: '2026-07-25',
    snapshot: const ActivitySnapshotSummary(
      steps: 11240,
      sleepMinutes: 431,
      activeCalories: 380,
      workouts: 1,
    ),
    vitality: vitality,
    vitalityAwarded: vitality,
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
  );
}

final _card = TechniqueCardSummary(
  id: 'card-1',
  ownerId: 'player-1',
  type: TechniqueType.hook,
  element: CreatureElement.earth,
  rarity: TechniqueRarity.uncommon,
  baseDamage: 18,
  speed: 50,
  staminaCost: 12,
  critChance: 0.05,
  effect: TechniqueEffect.none,
  effectValue: 0,
  quality: 44,
  createdAt: DateTime.parse('2026-07-25T20:00:00Z'),
);
