import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gochya_companion/app/app.dart';
import 'package:gochya_companion/app/gochya_loader.dart';
import 'package:gochya_companion/app/theme.dart';
import 'package:gochya_companion/core/models/activity_models.dart';
import 'package:gochya_companion/core/models/profile_models.dart';
import 'package:gochya_companion/features/activity/activity_repository.dart';
import 'package:gochya_companion/features/dojo_upsell/dojo_upsell_screen.dart';
import 'package:gochya_companion/features/home/need_indicator.dart';
import 'package:gochya_companion/features/home/profile_repository.dart';
import 'package:gochya_companion/features/home/symbiosis_card.dart';
import 'package:gochya_companion/features/profile/profile_screen.dart';
import 'package:gochya_companion/dev/demo_mode.dart';

/// `ART_BIBLE.md` §4 requires the layout to survive the font scale both
/// platforms allow, and names 200% as the limit to check. A widget test is the
/// cheapest place to hold that line: an overflowing RenderFlex throws, so any
/// screen that cannot take the larger type fails here rather than on a
/// player's phone.
const _maxScale = TextScaler.linear(2);

void main() {
  testWidgets('every tab survives 200% type', (tester) async {
    // Demo mode already fakes every repository, so one walk covers all five
    // tabs instead of hand-building a fake per screen.
    tester.platformDispatcher.textScaleFactorTestValue = 2;
    addTearDown(tester.platformDispatcher.clearTextScaleFactorTestValue);
    await tester.binding.setSurfaceSize(const Size(390, 4000));
    addTearDown(() => tester.binding.setSurfaceSize(null));

    await tester.pumpWidget(const DemoPlayerScope(child: GochyaApp()));
    await tester.pumpAndSettle();
    expect(tester.takeException(), isNull, reason: 'Главная overflows at 200%');

    for (final tab in ['Магазин', 'PvP', 'Бридинг', 'Профиль', 'Главная']) {
      await tester.tap(find.text(tab));
      await tester.pumpAndSettle();
      expect(tester.takeException(), isNull, reason: '$tab overflows at 200%');
    }
  });

  testWidgets('the loader survives 200% type', (tester) async {
    await _pump(
      tester,
      _app(const Scaffold(body: GochyaLoader(caption: 'Считаем Vitality…'))),
    );
  });

  testWidgets('a low need survives 200% type', (tester) async {
    await _pump(
      tester,
      _app(
        const Scaffold(
          body: NeedIndicator(
            label: 'Настроение',
            value: 8,
            color: GochyaColors.mood,
          ),
        ),
      ),
    );
  });

  testWidgets('the dojo upsell survives 200% type', (tester) async {
    await _pump(tester, _app(const DojoUpsellScreen()));
  });

  testWidgets('the profile survives 200% type', (tester) async {
    await _pump(
      tester,
      ProviderScope(
        overrides: [
          profileRepositoryProvider.overrideWithValue(
            const _FakeProfileRepository(),
          ),
        ],
        child: _app(const ProfileScreen(accessToken: 'access-token')),
      ),
    );
  });

  testWidgets('the symbiosis rings survive 200% type', (tester) async {
    await _pump(
      tester,
      ProviderScope(
        overrides: [
          activityRepositoryProvider.overrideWithValue(
            const _FakeActivityRepository(),
          ),
        ],
        child: _app(
          const Scaffold(body: SymbiosisCard(accessToken: 'access-token')),
        ),
      ),
    );
  });
}

Widget _app(Widget home) {
  return MaterialApp(
    theme: buildGochyaTheme(),
    home: MediaQuery(
      data: const MediaQueryData(
        textScaler: _maxScale,
        disableAnimations: true,
      ),
      child: home,
    ),
  );
}

Future<void> _pump(WidgetTester tester, Widget root) async {
  // A real phone width (390) with a tall canvas: height is not the defect being
  // hunted, horizontal overflow at large type is.
  await tester.binding.setSurfaceSize(const Size(390, 4000));
  addTearDown(() => tester.binding.setSurfaceSize(null));
  await tester.pumpWidget(root);
  await tester.pumpAndSettle();
  expect(tester.takeException(), isNull);
}

class _FakeProfileRepository implements ProfileRepository {
  const _FakeProfileRepository();

  @override
  Future<HomeSnapshot> loadHome(String accessToken) async {
    return HomeSnapshot(
      profile: PlayerProfile(
        id: 'player-1',
        username: 'nika_demo',
        displayName: 'Ника',
        createdAt: DateTime.parse('2026-07-20T10:00:00Z'),
        streakDays: 8,
        activePetId: 'pet-1',
      ),
      pets: [
        PetSummary(
          id: 'pet-1',
          ownerId: 'player-1',
          genome: const {'element': 2},
          name: 'Моти',
          stage: 'baby',
          level: 4,
          xp: 320,
          needs: const PetNeeds(hunger: 80, energy: 70, hygiene: 60, mood: 50),
          stats: const PetStats(
            strength: 2,
            agility: 3,
            endurance: 4,
            focus: 5,
          ),
          generation: 1,
          isActive: true,
          createdAt: DateTime.parse('2026-07-20T10:00:00Z'),
          isWeak: false,
          careRevision: 9,
          needsUpdatedAt: DateTime.parse('2026-07-26T12:00:00Z'),
        ),
      ],
    );
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
        date: '2026-07-26',
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
        updatedAt: DateTime.parse('2026-07-26T20:00:00Z'),
      ),
    ];
  }

  @override
  Future<ActivityRewardResult> claimReward(String accessToken) =>
      throw UnsupportedError('this test never claims');
}
