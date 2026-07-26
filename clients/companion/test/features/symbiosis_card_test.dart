import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gochya_companion/app/theme.dart';
import 'package:gochya_companion/core/models/activity_models.dart';
import 'package:gochya_companion/features/activity/activity_repository.dart';
import 'package:gochya_companion/features/activity/activity_rings.dart';
import 'package:gochya_companion/features/home/symbiosis_card.dart';

void main() {
  testWidgets('shows the day against the goals the server set', (tester) async {
    await _pump(tester, _FakeRepository());

    expect(find.text('11240 / 9000'), findsOneWidget);
    expect(find.text('7.2 / 7.5 ч'), findsOneWidget);
    expect(find.text('380 / 420'), findsOneWidget);
    // Vitality is the aggregate the pet actually grows on, so it sits in the
    // middle of the rings.
    expect(find.text('118'), findsOneWidget);
  });

  testWidgets('leaves the rings empty with no health source', (tester) async {
    await _pump(tester, _FakeRepository(days: const []));

    // The client never invents activity numbers: an empty week reads as empty,
    // not as a zero-scoring day.
    expect(find.text('—'), findsWidgets);
    expect(find.textContaining('Подключите источник здоровья'), findsOneWidget);
    expect(find.text('11240 / 9000'), findsNothing);
  });

  testWidgets('says what happened when activity cannot be read', (
    tester,
  ) async {
    await _pump(tester, _FakeRepository(fail: true));

    expect(
      find.textContaining('Не удалось прочитать активность'),
      findsOneWidget,
    );
  });

  testWidgets('clamps the arc at a full turn but keeps the real ratio', (
    tester,
  ) async {
    await _pump(tester, _FakeRepository(steps: 18000));

    final rings = tester.widget<ActivityRings>(find.byType(ActivityRings));
    expect(rings.steps.sweep, 1.0);
    expect(rings.steps.ratio, greaterThan(1.0));
  });

  testWidgets('announces every ring to a screen reader', (tester) async {
    await _pump(tester, _FakeRepository());

    final handle = tester.ensureSemantics();
    expect(
      find.bySemanticsLabel(RegExp(r'Шаги 11240 из 9000')),
      findsOneWidget,
    );
    expect(
      find.bySemanticsLabel(RegExp(r'Vitality 118 из 150')),
      findsOneWidget,
    );
    handle.dispose();
  });

  test('a zero goal never divides by zero', () {
    const ring = RingProgress(
      value: 500,
      goal: 0,
      color: GochyaRingColors.steps,
    );

    expect(ring.ratio, 0);
    expect(ring.sweep, 0);
  });
}

Future<void> _pump(WidgetTester tester, ActivityRepository repository) async {
  await tester.binding.setSurfaceSize(const Size(1000, 1400));
  addTearDown(() => tester.binding.setSurfaceSize(null));
  await tester.pumpWidget(
    ProviderScope(
      overrides: [activityRepositoryProvider.overrideWithValue(repository)],
      child: MaterialApp(
        theme: buildGochyaTheme(),
        home: const Scaffold(body: SymbiosisCard(accessToken: 'access-token')),
      ),
    ),
  );
  await tester.pumpAndSettle();
}

class _FakeRepository implements ActivityRepository {
  _FakeRepository({this.days, this.fail = false, this.steps = 11240});

  final List<DailyActivity>? days;
  final bool fail;
  final int steps;

  @override
  Future<List<DailyActivity>> week(String accessToken) async {
    if (fail) {
      throw StateError('activity unavailable');
    }
    return days ?? [_day(steps)];
  }

  @override
  Future<ActivityRewardResult> claimReward(String accessToken) =>
      throw UnsupportedError('this test never claims');
}

DailyActivity _day(int steps) {
  return DailyActivity(
    date: '2026-07-26',
    snapshot: ActivitySnapshotSummary(
      steps: steps,
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
    updatedAt: DateTime.parse('2026-07-26T20:00:00Z'),
  );
}
