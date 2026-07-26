import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gochya_companion/app/theme.dart';
import 'package:gochya_companion/core/models/battle_models.dart';
import 'package:gochya_companion/core/models/profile_models.dart';
import 'package:gochya_companion/core/models/technique_models.dart';
import 'package:gochya_companion/core/network/gochya_api_client.dart';
import 'package:gochya_companion/features/battle/battle_repository.dart';
import 'package:gochya_companion/features/battle/battle_screen.dart';
import 'package:gochya_companion/features/home/profile_repository.dart';
import 'package:gochya_companion/features/techniques/technique_repository.dart';

void main() {
  testWidgets('renders the server replay and claims its reward once', (
    tester,
  ) async {
    final battle = _FakeBattleRepository();
    await _pumpBattle(tester, battle: battle);

    expect(find.textContaining('К бою готов'), findsOneWidget);

    await _tap(tester, find.text('Найти бой'));
    await tester.pumpAndSettle();

    expect(battle.queueCalls, 1);
    expect(find.text('Победа'), findsWidgets);
    expect(find.textContaining('HP 74 : 0'), findsOneWidget);

    await _tap(tester, find.text('Забрать награду'));
    await tester.pumpAndSettle();

    expect(battle.confirmedMatchId, 'match-1');
    expect(find.textContaining('+30 Koins'), findsOneWidget);
    expect(find.textContaining('Новая карта'), findsOneWidget);
    expect(find.text('Забрать награду'), findsNothing);
  });

  testWidgets('blocks queueing until a loadout exists', (tester) async {
    final battle = _FakeBattleRepository();
    await _pumpBattle(tester, battle: battle, hasLoadout: false);

    expect(find.text('Лоадаут не собран'), findsOneWidget);
    expect(
      tester
          .widget<FilledButton>(find.widgetWithText(FilledButton, 'Найти бой'))
          .onPressed,
      isNull,
    );
    expect(battle.queueCalls, 0);
  });

  testWidgets('reuses the queue idempotency key after an uncertain result', (
    tester,
  ) async {
    final battle = _FakeBattleRepository(
      queueError: const ApiException(
        code: 'network_error',
        message: 'connection lost after send',
      ),
    );
    await _pumpBattle(tester, battle: battle);

    await _tap(tester, find.text('Найти бой'));
    await tester.pumpAndSettle();

    expect(
      find.textContaining('тот же ключ вернёт тот же бой'),
      findsOneWidget,
    );

    await _tap(tester, find.text('Повторить поиск'));
    await tester.pumpAndSettle();

    expect(battle.idempotencyKeys, hasLength(2));
    expect(battle.idempotencyKeys.first, battle.idempotencyKeys.last);
  });
}

Future<void> _tap(WidgetTester tester, Finder finder) async {
  await tester.ensureVisible(finder);
  await tester.pumpAndSettle();
  await tester.tap(finder);
  await tester.pump();
}

Future<void> _pumpBattle(
  WidgetTester tester, {
  required BattleRepository battle,
  bool hasLoadout = true,
}) async {
  await tester.binding.setSurfaceSize(const Size(1000, 3000));
  addTearDown(() => tester.binding.setSurfaceSize(null));
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        battleRepositoryProvider.overrideWithValue(battle),
        techniqueRepositoryProvider.overrideWithValue(
          _FakeTechniqueRepository(hasLoadout ? _loadout : null),
        ),
        profileRepositoryProvider.overrideWithValue(
          const _FakeProfileRepository(),
        ),
      ],
      child: MaterialApp(
        theme: buildGochyaTheme(),
        home: const BattleScreen(accessToken: 'access-token'),
      ),
    ),
  );
  await tester.pumpAndSettle();
}

class _FakeBattleRepository implements BattleRepository {
  _FakeBattleRepository({this.queueError});

  final Object? queueError;
  final idempotencyKeys = <String>[];
  var queueCalls = 0;
  String? confirmedMatchId;

  @override
  Future<MatchReplay> queueCasual({
    required String accessToken,
    required String idempotencyKey,
  }) async {
    queueCalls++;
    idempotencyKeys.add(idempotencyKey);
    if (queueError case final error?) {
      throw error;
    }
    return _replay;
  }

  @override
  Future<MatchConfirmation> confirm({
    required String accessToken,
    required String matchId,
  }) async {
    confirmedMatchId = matchId;
    return MatchConfirmation(
      matchId: matchId,
      outcome: MatchOutcome.win,
      rewards: const [MatchReward(currency: 'koins', amount: 30)],
      card: _card,
      confirmedAt: DateTime.parse('2026-07-25T10:00:05Z'),
    );
  }

  @override
  Future<List<MatchSummary>> history(String accessToken) async => const [];
}

class _FakeTechniqueRepository implements TechniqueRepository {
  const _FakeTechniqueRepository(this.loadout);

  final PetLoadout? loadout;

  @override
  Future<LoadoutSnapshot> load(String accessToken) async {
    return LoadoutSnapshot(cards: [_card], loadout: loadout);
  }

  @override
  Future<PetLoadout> equip({
    required String accessToken,
    required List<String> cardIds,
    required int signatureIdx,
    required String idempotencyKey,
  }) => throw UnsupportedError('the battle screen never equips cards');
}

class _FakeProfileRepository implements ProfileRepository {
  const _FakeProfileRepository();

  @override
  Future<HomeSnapshot> loadHome(String accessToken) async {
    return HomeSnapshot(profile: _profile, pets: const []);
  }

  @override
  Future<LineageTree> loadLineage(String accessToken, String petId) =>
      throw UnsupportedError('the battle screen never reads lineage');
}

final _profile = PlayerProfile(
  id: 'player-1',
  username: 'nika',
  createdAt: DateTime.parse('2026-07-20T10:00:00Z'),
  streakDays: 3,
  activePetId: 'pet-1',
);

final _card = TechniqueCardSummary(
  id: 'card-1',
  ownerId: 'player-1',
  type: TechniqueType.jab,
  element: CreatureElement.earth,
  rarity: TechniqueRarity.rare,
  baseDamage: 18,
  speed: 50,
  staminaCost: 12,
  critChance: 0.05,
  effect: TechniqueEffect.none,
  effectValue: 0,
  quality: 55,
  createdAt: DateTime.parse('2026-07-24T10:00:00Z'),
);

final _loadout = PetLoadout(
  petId: 'pet-1',
  cardIds: const ['card-1', 'card-2', 'card-3', 'card-4', 'card-5'],
  signatureIdx: 0,
  revision: 7,
  updatedAt: DateTime.parse('2026-07-25T09:00:00Z'),
);

final _replay = MatchReplay(
  id: 'match-1',
  playerAId: 'player-1',
  playerBId: 'player-2',
  elementA: CreatureElement.earth,
  elementB: CreatureElement.fire,
  mode: 'casual',
  loadoutRevisionA: 7,
  loadoutRevisionB: 4,
  winner: 'a',
  rounds: const [
    MatchRound(
      cardAIdx: 0,
      cardBIdx: 2,
      damageAToB: 18,
      damageBToA: 11,
      effect: TechniqueEffect.none,
      effectValue: 0,
    ),
  ],
  finalHpA: 74,
  finalHpB: 0,
  seed: 42,
  createdAt: DateTime.parse('2026-07-25T10:00:00Z'),
);
