import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gochya_companion/app/theme.dart';
import 'package:gochya_companion/core/models/profile_models.dart';
import 'package:gochya_companion/features/home/profile_repository.dart';
import 'package:gochya_companion/features/profile/profile_screen.dart';

void main() {
  testWidgets('shows the player and every pet they own', (tester) async {
    await _pump(tester, _FakeRepository());

    expect(find.text('Ника'), findsOneWidget);
    expect(find.text('@nika'), findsOneWidget);
    expect(find.text('Питомцы · 2'), findsOneWidget);
    expect(find.text('Моти'), findsOneWidget);
    expect(find.text('Аква'), findsOneWidget);
    expect(find.textContaining('Земля · Малыш · уровень 4'), findsOneWidget);
  });

  testWidgets('marks which pet is the active one', (tester) async {
    await _pump(tester, _FakeRepository());

    expect(find.text('активный'), findsOneWidget);
  });

  testWidgets('never invents data the server does not send', (tester) async {
    await _pump(tester, _FakeRepository());

    // `UX_UI.md` §7.5 asks for league, rating, friends and a battle pass, and
    // none of them has an endpoint. An empty "Лига —" row would promise
    // something the backend cannot deliver, so nothing is drawn at all.
    for (final absent in ['Лига', 'Рейтинг', 'Друзья', 'Battle Pass']) {
      expect(
        find.textContaining(absent),
        findsNothing,
        reason: '$absent has no server data and must not be shown',
      );
    }
  });

  testWidgets('says so plainly when there are no pets', (tester) async {
    await _pump(tester, _FakeRepository(pets: const []));

    expect(find.text('Питомцы · 0'), findsOneWidget);
    expect(find.textContaining('Питомцев пока нет'), findsOneWidget);
  });

  testWidgets('offers retry and sign-out when the profile cannot load', (
    tester,
  ) async {
    await _pump(tester, _FakeRepository(fail: true));

    expect(find.textContaining('Не удалось прочитать профиль'), findsOneWidget);
    expect(find.widgetWithText(FilledButton, 'Повторить'), findsOneWidget);
    expect(find.widgetWithText(TextButton, 'Выйти'), findsOneWidget);
  });
}

Future<void> _pump(WidgetTester tester, ProfileRepository repository) async {
  await tester.binding.setSurfaceSize(const Size(1000, 2000));
  addTearDown(() => tester.binding.setSurfaceSize(null));
  await tester.pumpWidget(
    ProviderScope(
      overrides: [profileRepositoryProvider.overrideWithValue(repository)],
      child: MaterialApp(
        theme: buildGochyaTheme(),
        home: const ProfileScreen(accessToken: 'access-token'),
      ),
    ),
  );
  await tester.pumpAndSettle();
}

class _FakeRepository implements ProfileRepository {
  const _FakeRepository({this.pets, this.fail = false});

  final List<PetSummary>? pets;
  final bool fail;

  @override
  Future<HomeSnapshot> loadHome(String accessToken) async {
    if (fail) {
      throw StateError('profile unavailable');
    }
    return HomeSnapshot(
      profile: _profile,
      pets:
          pets ??
          [_pet('pet-1', 'Моти', 2, true), _pet('pet-2', 'Аква', 1, false)],
    );
  }

  @override
  Future<LineageTree> loadLineage(String accessToken, String petId) =>
      throw UnsupportedError('this test never opens lineage');
}

final _profile = PlayerProfile(
  id: 'player-1',
  username: 'nika',
  displayName: 'Ника',
  createdAt: DateTime.parse('2026-07-20T10:00:00Z'),
  streakDays: 8,
  activePetId: 'pet-1',
);

PetSummary _pet(String id, String name, int element, bool active) {
  return PetSummary(
    id: id,
    ownerId: 'player-1',
    genome: {'element': element},
    name: name,
    stage: 'baby',
    level: 4,
    xp: 320,
    needs: const PetNeeds(hunger: 80, energy: 70, hygiene: 60, mood: 50),
    stats: const PetStats(strength: 2, agility: 3, endurance: 4, focus: 5),
    generation: 1,
    isActive: active,
    createdAt: DateTime.parse('2026-07-20T10:00:00Z'),
    isWeak: false,
    careRevision: 9,
    needsUpdatedAt: DateTime.parse('2026-07-26T12:00:00Z'),
  );
}
