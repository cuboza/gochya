import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gochya_companion/app/app.dart';
import 'package:gochya_companion/core/models/profile_models.dart';
import 'package:gochya_companion/core/network/gochya_api_client.dart';
import 'package:gochya_companion/core/session/session_store.dart';
import 'package:gochya_companion/features/home/profile_repository.dart';
import 'package:gochya_companion/features/session/session_controller.dart';

void main() {
  testWidgets('shows a safe signed-out state without manual token input', (
    tester,
  ) async {
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          sessionStoreProvider.overrideWithValue(_MemorySessionStore()),
        ],
        child: const GochyaApp(),
      ),
    );
    await tester.pumpAndSettle();

    expect(
      find.text(
        'Защищённая сессия не найдена. Вход через провайдера '
        'появится после подключения production OAuth-конфигурации.',
      ),
      findsOneWidget,
    );
    expect(find.text('Токены не вводятся вручную'), findsOneWidget);
  });

  testWidgets('renders profile, active pet, needs, and lineage', (
    tester,
  ) async {
    final store = _MemorySessionStore(tokens: _tokens);
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          sessionStoreProvider.overrideWithValue(store),
          profileRepositoryProvider.overrideWithValue(_FakeRepository()),
        ],
        child: const GochyaApp(),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('Привет, Ника'), findsOneWidget);
    expect(find.text('Моти'), findsOneWidget);
    expect(find.text('81%'), findsOneWidget);
    await tester.scrollUntilVisible(
      find.text('Открыть родословную'),
      300,
      scrollable: find.byType(Scrollable).first,
    );
    expect(find.text('Открыть родословную'), findsOneWidget);

    await tester.tap(find.text('Открыть родословную'));
    await tester.pumpAndSettle();

    expect(find.text('Родословная · Моти'), findsOneWidget);
    expect(find.text('Родители'), findsOneWidget);
    expect(find.text('Предок'), findsOneWidget);
  });

  testWidgets('expired API session can be cleared safely', (tester) async {
    final store = _MemorySessionStore(tokens: _tokens);
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          sessionStoreProvider.overrideWithValue(store),
          profileRepositoryProvider.overrideWithValue(
            const _UnauthorizedRepository(),
          ),
        ],
        child: const GochyaApp(),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('Сессия истекла'), findsOneWidget);
    await tester.tap(find.text('Очистить сессию'));
    await tester.pumpAndSettle();

    expect(store.wasCleared, isTrue);
    expect(find.text('Токены не вводятся вручную'), findsOneWidget);
  });
}

const _tokens = SessionTokens(
  accessToken: 'access-token',
  refreshToken: 'refresh-token',
);

class _MemorySessionStore implements SessionStore {
  _MemorySessionStore({this.tokens});

  SessionTokens? tokens;
  bool wasCleared = false;

  @override
  Future<void> clear() async {
    tokens = null;
    wasCleared = true;
  }

  @override
  Future<SessionTokens?> read() async => tokens;

  @override
  Future<void> write(SessionTokens tokens) async {
    this.tokens = tokens;
  }
}

class _FakeRepository implements ProfileRepository {
  @override
  Future<HomeSnapshot> loadHome(String accessToken) async {
    return HomeSnapshot(profile: _profile, pets: [_pet]);
  }

  @override
  Future<LineageTree> loadLineage(String accessToken, String petId) async {
    return const LineageTree(
      rootId: 'pet-1',
      maxDepth: 3,
      truncated: false,
      nodes: [
        LineageNode(
          id: 'pet-1',
          genome: {'element': 'Earth'},
          name: 'Моти',
          stage: 'baby',
          level: 4,
          generation: 1,
          mutatedGenes: 1,
          parentAId: 'pet-a',
          parentBId: 'pet-b',
          ancestorDepth: 0,
        ),
        LineageNode(
          id: 'pet-a',
          genome: {'element': 'Water'},
          stage: 'adult',
          level: 18,
          generation: 0,
          mutatedGenes: 0,
          ancestorDepth: 1,
        ),
      ],
    );
  }
}

class _UnauthorizedRepository implements ProfileRepository {
  const _UnauthorizedRepository();

  @override
  Future<HomeSnapshot> loadHome(String accessToken) {
    throw const ApiException(
      statusCode: 401,
      code: 'token_expired',
      message: 'expired',
    );
  }

  @override
  Future<LineageTree> loadLineage(String accessToken, String petId) {
    throw UnimplementedError();
  }
}

final _profile = PlayerProfile(
  id: 'player-1',
  username: 'nika',
  displayName: 'Ника',
  createdAt: DateTime.utc(2026, 7, 20),
  streakDays: 8,
  activePetId: 'pet-1',
);

final _pet = PetSummary(
  id: 'pet-1',
  ownerId: 'player-1',
  genome: const {'element': 'Earth'},
  name: 'Моти',
  stage: 'baby',
  level: 4,
  xp: 320,
  needs: const PetNeeds(hunger: 81, energy: 72, hygiene: 65, mood: 94),
  stats: const PetStats(strength: 2, agility: 3, endurance: 4, focus: 5),
  generation: 1,
  isActive: true,
  createdAt: DateTime.utc(2026, 7, 20),
  parentAId: 'pet-a',
  parentBId: 'pet-b',
  isWeak: false,
  careRevision: 9,
  needsUpdatedAt: DateTime.utc(2026, 7, 24),
);
