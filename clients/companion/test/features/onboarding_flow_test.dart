import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gochya_companion/app/app.dart';
import 'package:gochya_companion/core/models/onboarding_models.dart';
import 'package:gochya_companion/core/models/profile_models.dart';
import 'package:gochya_companion/core/network/gochya_api_client.dart';
import 'package:gochya_companion/core/session/session_store.dart';
import 'package:gochya_companion/features/home/profile_repository.dart';
import 'package:gochya_companion/features/onboarding/onboarding_repository.dart';
import 'package:gochya_companion/features/session/session_controller.dart';

void main() {
  testWidgets('new player selects, resumes, and hatches a starter egg', (
    tester,
  ) async {
    final flow = _FlowState();
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          sessionStoreProvider.overrideWithValue(
            _MemorySessionStore(tokens: _tokens),
          ),
          profileRepositoryProvider.overrideWithValue(
            _FlowProfileRepository(flow),
          ),
          onboardingRepositoryProvider.overrideWithValue(
            _FlowOnboardingRepository(flow),
          ),
        ],
        child: const GochyaApp(),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('Создать первого питомца'), findsOneWidget);
    await tester.tap(find.text('Создать первого питомца'));
    await tester.pumpAndSettle();

    expect(find.text('Сколько тебе лет?'), findsOneWidget);
    await tester.tap(find.text('Выбрать дату рождения'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Выбрать'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Продолжить'));
    await tester.pumpAndSettle();

    expect(find.text('Выбери стихию яйца'), findsOneWidget);
    await tester.tap(find.text('Вода'));
    await tester.pump();
    await tester.tap(find.text('Выбрать яйцо'));
    await tester.pumpAndSettle();

    expect(flow.ageRequests, 1);
    expect(flow.selectedElement, StarterElement.water);
    expect(find.text('Яйцо готово!'), findsOneWidget);
    await tester.tap(find.text('Вылупить питомца'));
    await tester.pumpAndSettle();

    expect(flow.hatchRequests, 1);
    expect(find.text('Луми'), findsOneWidget);
    expect(find.text('Состояние'), findsOneWidget);
  });

  testWidgets('underage result stops before starter selection', (tester) async {
    final flow = _FlowState(restricted: true);
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          sessionStoreProvider.overrideWithValue(
            _MemorySessionStore(tokens: _tokens),
          ),
          profileRepositoryProvider.overrideWithValue(
            _FlowProfileRepository(flow),
          ),
          onboardingRepositoryProvider.overrideWithValue(
            _FlowOnboardingRepository(flow),
          ),
        ],
        child: const GochyaApp(),
      ),
    );
    await tester.pumpAndSettle();
    await tester.tap(find.text('Создать первого питомца'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Выбрать дату рождения'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Выбрать'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Продолжить'));
    await tester.pumpAndSettle();

    expect(find.text('Нужно согласие родителя'), findsOneWidget);
    expect(find.text('Выбери стихию яйца'), findsNothing);
    expect(flow.selectedElement, isNull);
  });

  testWidgets('onboarding retries reuse the same idempotency keys', (
    tester,
  ) async {
    final flow = _FlowState(ageFailures: 1, starterFailures: 1);
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          sessionStoreProvider.overrideWithValue(
            _MemorySessionStore(tokens: _tokens),
          ),
          profileRepositoryProvider.overrideWithValue(
            _FlowProfileRepository(flow),
          ),
          onboardingRepositoryProvider.overrideWithValue(
            _FlowOnboardingRepository(flow),
          ),
        ],
        child: const GochyaApp(),
      ),
    );
    await tester.pumpAndSettle();
    await tester.tap(find.text('Создать первого питомца'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Выбрать дату рождения'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Выбрать'));
    await tester.pumpAndSettle();

    await tester.tap(find.text('Продолжить'));
    await tester.pumpAndSettle();
    expect(find.textContaining('Не удалось сохранить выбор'), findsOneWidget);
    await tester.tap(find.text('Продолжить'));
    await tester.pumpAndSettle();

    await tester.tap(find.text('Земля'));
    await tester.pump();
    await tester.tap(find.text('Выбрать яйцо'));
    await tester.pumpAndSettle();
    expect(find.textContaining('Не удалось сохранить выбор'), findsOneWidget);
    await tester.tap(find.text('Выбрать яйцо'));
    await tester.pumpAndSettle();

    expect(flow.ageKeys, hasLength(2));
    expect(flow.ageKeys.first, flow.ageKeys.last);
    expect(flow.starterKeys, hasLength(2));
    expect(flow.starterKeys.first, flow.starterKeys.last);
    expect(
      flow.ageKeys.first,
      matches(
        RegExp(
          r'^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-'
          r'[89ab][0-9a-f]{3}-[0-9a-f]{12}$',
        ),
      ),
    );
  });
}

const _tokens = SessionTokens(
  accessToken: 'access-token',
  refreshToken: 'refresh-token',
);

class _FlowState {
  _FlowState({
    this.restricted = false,
    this.ageFailures = 0,
    this.starterFailures = 0,
  });

  final bool restricted;
  int ageFailures;
  int starterFailures;
  var ageRequests = 0;
  var hatchRequests = 0;
  var starterSelected = false;
  var hatched = false;
  StarterElement? selectedElement;
  final ageKeys = <String>[];
  final starterKeys = <String>[];

  final egg = EggSummary(
    id: 'egg-1',
    ownerId: 'player-1',
    origin: 'starter',
    genome: const {'element': 1},
    incubateUntil: DateTime.utc(2020),
    mutatedGenes: 0,
    createdAt: DateTime.utc(2020),
  );
}

class _FlowOnboardingRepository implements OnboardingRepository {
  const _FlowOnboardingRepository(this.flow);

  final _FlowState flow;

  @override
  Future<HatchedPet> hatchEgg(String accessToken, String eggId) async {
    flow.hatchRequests++;
    flow.hatched = true;
    return const HatchedPet(
      id: 'pet-1',
      ownerId: 'player-1',
      stage: 'baby',
      isActive: true,
    );
  }

  @override
  Future<List<EggSummary>> loadEggs(String accessToken) async {
    if (flow.starterSelected && !flow.hatched) {
      return [flow.egg];
    }
    return const [];
  }

  @override
  Future<AgeGateResult> recordAgeGate({
    required String accessToken,
    required DateTime birthDate,
    required String idempotencyKey,
  }) async {
    flow.ageRequests++;
    flow.ageKeys.add(idempotencyKey);
    if (flow.ageFailures > 0) {
      flow.ageFailures--;
      throw const ApiException(code: 'network_error', message: 'offline');
    }
    return AgeGateResult(
      status: flow.restricted ? 'parental_consent_required' : 'eligible',
      coppaRestricted: flow.restricted,
      recordedAt: DateTime.utc(2026, 7, 25),
    );
  }

  @override
  Future<StarterEggResult> selectStarterEgg({
    required String accessToken,
    required StarterElement element,
    required String idempotencyKey,
  }) async {
    flow.starterKeys.add(idempotencyKey);
    if (flow.starterFailures > 0) {
      flow.starterFailures--;
      throw const ApiException(code: 'network_error', message: 'offline');
    }
    flow.selectedElement = element;
    flow.starterSelected = true;
    return StarterEggResult(
      eggId: flow.egg.id,
      element: element,
      incubateUntil: flow.egg.incubateUntil,
    );
  }
}

class _FlowProfileRepository implements ProfileRepository {
  const _FlowProfileRepository(this.flow);

  final _FlowState flow;

  @override
  Future<HomeSnapshot> loadHome(String accessToken) async {
    return HomeSnapshot(
      profile: _profile,
      pets: flow.hatched ? [_pet] : const [],
    );
  }

  @override
  Future<LineageTree> loadLineage(String accessToken, String petId) {
    throw UnimplementedError();
  }
}

class _MemorySessionStore implements SessionStore {
  _MemorySessionStore({this.tokens});

  SessionTokens? tokens;

  @override
  Future<void> clear() async {
    tokens = null;
  }

  @override
  Future<SessionTokens?> read() async => tokens;

  @override
  Future<void> write(SessionTokens tokens) async {
    this.tokens = tokens;
  }
}

final _profile = PlayerProfile(
  id: 'player-1',
  username: 'nika',
  displayName: 'Ника',
  createdAt: DateTime.utc(2026, 7, 20),
  streakDays: 0,
  activePetId: 'pet-1',
);

final _pet = PetSummary(
  id: 'pet-1',
  ownerId: 'player-1',
  genome: const {'element': 1},
  name: 'Луми',
  stage: 'baby',
  level: 1,
  xp: 0,
  needs: const PetNeeds(hunger: 100, energy: 100, hygiene: 100, mood: 100),
  stats: const PetStats(strength: 1, agility: 1, endurance: 1, focus: 1),
  generation: 0,
  isActive: true,
  createdAt: DateTime.utc(2026, 7, 25),
  isWeak: false,
  careRevision: 0,
  needsUpdatedAt: DateTime.utc(2026, 7, 25),
);
