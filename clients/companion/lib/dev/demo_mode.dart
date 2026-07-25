import 'dart:math';

import 'package:flutter/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../core/models/activity_models.dart';
import '../core/models/battle_models.dart';
import '../core/models/breeding_models.dart';
import '../core/models/care_models.dart';
import '../core/models/onboarding_models.dart';
import '../core/models/profile_models.dart';
import '../core/models/shop_models.dart';
import '../core/models/technique_models.dart';
import '../core/network/gochya_api_client.dart';
import '../core/session/session_store.dart';
import '../features/activity/activity_repository.dart';
import '../features/auth/auth_repository.dart';
import '../features/battle/battle_repository.dart';
import '../features/breeding/breeding_repository.dart';
import '../features/care/care_queue_store.dart';
import '../features/care/care_repository.dart';
import '../features/home/profile_repository.dart';
import '../features/session/session_controller.dart';
import '../features/shop/shop_repository.dart';
import '../features/techniques/technique_repository.dart';

class DemoPlayerScope extends StatefulWidget {
  const DemoPlayerScope({required this.child, super.key});

  final Widget child;

  @override
  State<DemoPlayerScope> createState() => _DemoPlayerScopeState();
}

class _DemoPlayerScopeState extends State<DemoPlayerScope> {
  late final _state = _DemoPlayerState();
  late final _sessionStore = _DemoSessionStore();
  late final _careQueueStore = _DemoCareQueueStore();
  late final _profileRepository = _DemoProfileRepository(_state);
  late final _careRepository = _DemoCareRepository(_state);
  late final _shopRepository = _DemoShopRepository(_state);
  late final _techniqueRepository = _DemoTechniqueRepository(_state);
  late final _battleRepository = _DemoBattleRepository(_state);
  late final _breedingRepository = _DemoBreedingRepository(_state);
  late final _activityRepository = _DemoActivityRepository(_state);

  @override
  Widget build(BuildContext context) {
    return ProviderScope(
      overrides: [
        sessionStoreProvider.overrideWithValue(_sessionStore),
        authRepositoryProvider.overrideWithValue(const _DemoAuthRepository()),
        careQueueStoreProvider.overrideWithValue(_careQueueStore),
        profileRepositoryProvider.overrideWithValue(_profileRepository),
        careRepositoryProvider.overrideWithValue(_careRepository),
        shopRepositoryProvider.overrideWithValue(_shopRepository),
        techniqueRepositoryProvider.overrideWithValue(_techniqueRepository),
        battleRepositoryProvider.overrideWithValue(_battleRepository),
        breedingRepositoryProvider.overrideWithValue(_breedingRepository),
        activityRepositoryProvider.overrideWithValue(_activityRepository),
      ],
      child: widget.child,
    );
  }
}

class _DemoPlayerState {
  _DemoPlayerState()
    : pet = PetSummary(
        id: 'demo-pet-1',
        ownerId: 'demo-player-1',
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
        parentAId: 'demo-pet-a',
        parentBId: 'demo-pet-b',
        isWeak: false,
        careRevision: 9,
        needsUpdatedAt: DateTime.utc(2026, 7, 25, 12),
      ),
      shop = const ShopSnapshot(
        catalog: _demoCatalog,
        inventory: ShopInventory(
          koins: 500,
          items: [
            OwnedShopItem(itemId: ShopItemId.apple, quantity: 3),
            OwnedShopItem(itemId: ShopItemId.soap, quantity: 1),
            OwnedShopItem(itemId: ShopItemId.loveCrystal, quantity: 1),
          ],
        ),
      );

  static final profile = PlayerProfile(
    id: 'demo-player-1',
    username: 'nika_demo',
    displayName: 'Ника',
    createdAt: DateTime.utc(2026, 7, 20),
    streakDays: 8,
    activePetId: 'demo-pet-1',
  );

  PetSummary pet;
  ShopSnapshot shop;
  PetLoadout? loadout;
  MatchReplay? lastMatch;
  ActivityRewardResult? claimedReward;
  final matchHistory = <MatchSummary>[];
  final eggs = <EggSummary>[];

  /// Adult breeding stock so the demo can exercise the eligibility gate.
  static final parents = <PetSummary>[
    _demoParent('demo-pet-a', 'Аква', 31),
    _demoParent('demo-pet-b', 'Искра', 34),
  ];

  void spendBreedingResources() {
    final inventory = shop.inventory;
    shop = ShopSnapshot(
      catalog: shop.catalog,
      inventory: ShopInventory(
        koins: inventory.koins - breedCostKoins,
        items: [
          for (final item in inventory.items)
            if (item.itemId != ShopItemId.loveCrystal)
              item
            else if (item.quantity > 1)
              OwnedShopItem(itemId: item.itemId, quantity: item.quantity - 1),
        ],
      ),
    );
  }

  CarePetSnapshot applyCare(CareOperation operation) {
    final current = pet;
    final needs = current.needs;
    final updatedNeeds = switch (operation) {
      CareOperation.feed => PetNeeds(
        hunger: min(100, needs.hunger + 12),
        energy: needs.energy,
        hygiene: needs.hygiene,
        mood: needs.mood,
      ),
      CareOperation.clean => PetNeeds(
        hunger: needs.hunger,
        energy: needs.energy,
        hygiene: min(100, needs.hygiene + 18),
        mood: needs.mood,
      ),
      CareOperation.play => PetNeeds(
        hunger: needs.hunger,
        energy: max(0, needs.energy - 5),
        hygiene: needs.hygiene,
        mood: min(100, needs.mood + 10),
      ),
      CareOperation.sleep => needs,
    };
    final now = DateTime.now().toUtc();
    final sleepingUntil = operation == CareOperation.sleep
        ? now.add(const Duration(hours: 8))
        : current.sleepingUntil;
    pet = PetSummary(
      id: current.id,
      ownerId: current.ownerId,
      genome: current.genome,
      name: current.name,
      stage: current.stage,
      level: current.level,
      xp: current.xp,
      needs: updatedNeeds,
      stats: current.stats,
      generation: current.generation,
      isActive: current.isActive,
      createdAt: current.createdAt,
      parentAId: current.parentAId,
      parentBId: current.parentBId,
      lastBredAt: current.lastBredAt,
      needsZeroSince: current.needsZeroSince,
      isWeak: false,
      careRevision: current.careRevision + 1,
      needsUpdatedAt: now,
      sleepingUntil: sleepingUntil,
    );
    return CarePetSnapshot(
      id: pet.id,
      needs: pet.needs,
      revision: pet.careRevision,
      isWeak: pet.isWeak,
      needsUpdatedAt: pet.needsUpdatedAt,
      sleepingUntil: pet.sleepingUntil,
    );
  }
}

class _DemoSessionStore implements SessionStore {
  SessionTokens? _tokens = const SessionTokens(
    accessToken: 'demo-access-token',
    refreshToken: 'demo-refresh-token',
  );

  @override
  Future<void> clear() async {
    _tokens = null;
  }

  @override
  Future<SessionTokens?> read() async => _tokens;

  @override
  Future<void> write(SessionTokens tokens) async {
    _tokens = tokens;
  }
}

class _DemoAuthRepository implements AuthRepository {
  const _DemoAuthRepository();

  @override
  bool get isGoogleSignInAvailable => false;

  @override
  Future<bool> isAppleSignInAvailable() async => false;

  @override
  Future<void> revokeSession(String refreshToken) async {}

  @override
  Future<SessionTokens> signInWithApple() =>
      throw UnsupportedError('provider login is disabled in demo player mode');

  @override
  Future<SessionTokens> signInWithGoogle() =>
      throw UnsupportedError('provider login is disabled in demo player mode');

  @override
  Future<void> signOutFromProvider() async {}
}

class _DemoProfileRepository implements ProfileRepository {
  const _DemoProfileRepository(this.state);

  final _DemoPlayerState state;

  @override
  Future<HomeSnapshot> loadHome(String accessToken) async {
    return HomeSnapshot(
      profile: _DemoPlayerState.profile,
      pets: [state.pet, ..._DemoPlayerState.parents],
    );
  }

  @override
  Future<LineageTree> loadLineage(String accessToken, String petId) async {
    return const LineageTree(
      rootId: 'demo-pet-1',
      maxDepth: 3,
      truncated: false,
      nodes: [
        LineageNode(
          id: 'demo-pet-1',
          genome: {'element': 'Earth'},
          name: 'Моти',
          stage: 'baby',
          level: 4,
          generation: 1,
          mutatedGenes: 1,
          parentAId: 'demo-pet-a',
          parentBId: 'demo-pet-b',
          ancestorDepth: 0,
        ),
        LineageNode(
          id: 'demo-pet-a',
          genome: {'element': 'Water'},
          name: 'Аква',
          stage: 'adult',
          level: 18,
          generation: 0,
          mutatedGenes: 0,
          ancestorDepth: 1,
        ),
        LineageNode(
          id: 'demo-pet-b',
          genome: {'element': 'Fire'},
          name: 'Искра',
          stage: 'adult',
          level: 17,
          generation: 0,
          mutatedGenes: 2,
          ancestorDepth: 1,
        ),
      ],
    );
  }
}

class _DemoCareRepository implements CareRepository {
  const _DemoCareRepository(this.state);

  final _DemoPlayerState state;

  @override
  Future<void> clearQueue() async {}

  @override
  Future<CareReconcileResult> reconcilePending({
    required String accountId,
    required String accessToken,
  }) async {
    return const CareReconcileResult(results: [], pendingCount: 0);
  }

  @override
  Future<CareSubmitResult> submit({
    required String accountId,
    required String accessToken,
    required String petId,
    required int canonicalRevision,
    required CareIntent intent,
  }) async {
    final snapshot = state.applyCare(intent.operation);
    final result = CareCommandResult(
      operationId: intent.operationId,
      status: CareCommandStatus.applied,
      snapshot: snapshot,
    );
    return CareSubmitResult(
      commandResult: result,
      canonicalSnapshot: snapshot,
      pendingCount: 0,
    );
  }
}

class _DemoShopRepository implements ShopRepository {
  const _DemoShopRepository(this.state);

  final _DemoPlayerState state;

  @override
  Future<ShopSnapshot> load(String accessToken) async => state.shop;

  @override
  Future<ShopPurchase> purchase({
    required String accessToken,
    required ShopItemId itemId,
    required int quantity,
    required String idempotencyKey,
  }) async {
    final matches = state.shop.catalog.items.where((item) => item.id == itemId);
    if (matches.length != 1) {
      throw const ApiException(
        statusCode: 400,
        code: 'shop_item_invalid',
        message: 'demo item is unavailable',
      );
    }
    final item = matches.single;
    final spent = item.unitPrice * quantity;
    if (spent > state.shop.inventory.koins) {
      throw const ApiException(
        statusCode: 409,
        code: 'insufficient_koins',
        message: 'not enough demo koins',
      );
    }
    final purchase = ShopPurchase(
      itemId: itemId,
      purchasedQuantity: quantity,
      itemQuantity: state.shop.inventory.quantityOf(itemId) + quantity,
      unitPriceKoins: item.unitPrice,
      koinsSpent: spent,
      koinsRemaining: state.shop.inventory.koins - spent,
      purchasedAt: DateTime.now().toUtc(),
    );
    state.shop = state.shop.applying(purchase);
    return purchase;
  }
}

class _DemoTechniqueRepository implements TechniqueRepository {
  const _DemoTechniqueRepository(this.state);

  final _DemoPlayerState state;

  @override
  Future<LoadoutSnapshot> load(String accessToken) async {
    return LoadoutSnapshot(cards: _demoCards, loadout: state.loadout);
  }

  @override
  Future<PetLoadout> equip({
    required String accessToken,
    required List<String> cardIds,
    required int signatureIdx,
    required String idempotencyKey,
  }) async {
    final loadout = PetLoadout(
      petId: state.pet.id,
      cardIds: List.unmodifiable(cardIds),
      signatureIdx: signatureIdx,
      revision: (state.loadout?.revision ?? 0) + 1,
      updatedAt: DateTime.now().toUtc(),
    );
    state.loadout = loadout;
    return loadout;
  }
}

class _DemoBattleRepository implements BattleRepository {
  const _DemoBattleRepository(this.state);

  final _DemoPlayerState state;

  @override
  Future<MatchReplay> queueCasual({
    required String accessToken,
    required String idempotencyKey,
  }) async {
    if (state.loadout == null) {
      throw const ApiException(
        statusCode: 409,
        code: 'loadout_required',
        message: 'demo loadout is not equipped',
      );
    }
    final now = DateTime.now().toUtc();
    final replay = MatchReplay(
      id: 'demo-match-${state.matchHistory.length + 1}',
      playerAId: _DemoPlayerState.profile.id,
      playerBId: 'demo-rival-7',
      mode: 'casual',
      loadoutRevisionA: state.loadout!.revision,
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
        MatchRound(
          cardAIdx: 3,
          cardBIdx: 1,
          damageAToB: 24,
          damageBToA: 9,
          effect: TechniqueEffect.stun,
          effectValue: 1,
        ),
        MatchRound(
          cardAIdx: 1,
          cardBIdx: 0,
          damageAToB: 21,
          damageBToA: 6,
          effect: TechniqueEffect.crit,
          effectValue: 1.5,
        ),
      ],
      finalHpA: 74,
      finalHpB: 0,
      seed: 42,
      createdAt: now,
    );
    state.lastMatch = replay;
    state.matchHistory.insert(
      0,
      MatchSummary(
        id: replay.id,
        opponentId: replay.playerBId,
        mode: 'casual',
        outcome: MatchOutcome.win,
        createdAt: now,
      ),
    );
    return replay;
  }

  @override
  Future<MatchConfirmation> confirm({
    required String accessToken,
    required String matchId,
  }) async {
    return MatchConfirmation(
      matchId: matchId,
      outcome: MatchOutcome.win,
      rewards: const [MatchReward(currency: 'koins', amount: 30)],
      card: _demoCards.last,
      confirmedAt: DateTime.now().toUtc(),
    );
  }

  @override
  Future<List<MatchSummary>> history(String accessToken) async {
    return List.unmodifiable(state.matchHistory);
  }
}

class _DemoBreedingRepository implements BreedingRepository {
  const _DemoBreedingRepository(this.state);

  final _DemoPlayerState state;

  @override
  Future<BreedingSnapshot> load(String accessToken) async {
    return BreedingSnapshot(
      pets: [state.pet, ..._DemoPlayerState.parents],
      eggs: List.unmodifiable(state.eggs),
      inventory: state.shop.inventory,
    );
  }

  @override
  Future<BreedingResult> breed({
    required String accessToken,
    required String parentAId,
    required String parentBId,
    required List<BreedingCatalyst> catalysts,
    required String idempotencyKey,
  }) async {
    if (!state.shop.inventory.items.any(
      (item) => item.itemId == ShopItemId.loveCrystal,
    )) {
      throw const ApiException(
        statusCode: 409,
        code: 'love_crystal_required',
        message: 'demo love crystal is missing',
      );
    }
    if (state.shop.inventory.koins < breedCostKoins) {
      throw const ApiException(
        statusCode: 409,
        code: 'insufficient_koins',
        message: 'not enough demo koins',
      );
    }
    final now = DateTime.now().toUtc();
    final egg = EggSummary(
      id: 'demo-egg-${state.eggs.length + 1}',
      ownerId: _DemoPlayerState.profile.id,
      origin: 'breeding',
      genome: const {'element': 'Steam'},
      parentAId: parentAId,
      parentBId: parentBId,
      incubateUntil: now.add(const Duration(hours: 6)),
      mutatedGenes: catalysts.contains(BreedingCatalyst.mutation) ? 3 : 0,
      createdAt: now,
    );
    state.spendBreedingResources();
    state.eggs.add(egg);
    return BreedingResult(eggId: egg.id, incubateUntil: egg.incubateUntil);
  }

  @override
  Future<HatchedPet> hatch({
    required String accessToken,
    required String eggId,
  }) async {
    state.eggs.removeWhere((egg) => egg.id == eggId);
    return HatchedPet(
      id: 'demo-pet-$eggId',
      ownerId: _DemoPlayerState.profile.id,
      stage: 'baby',
      isActive: false,
    );
  }
}

class _DemoActivityRepository implements ActivityRepository {
  const _DemoActivityRepository(this.state);

  final _DemoPlayerState state;

  @override
  Future<List<DailyActivity>> week(String accessToken) async => _demoWeek;

  @override
  Future<ActivityRewardResult> claimReward(String accessToken) async {
    final alreadyClaimed = state.claimedReward != null;
    final reward = ActivityRewardResult(
      date: _demoWeek.first.date,
      card: _demoCards.last,
      awarded: !alreadyClaimed,
    );
    state.claimedReward = reward;
    return reward;
  }
}

class _DemoCareQueueStore implements CareQueueStore {
  CareQueue? _queue;

  @override
  Future<void> clear() async {
    _queue = null;
  }

  @override
  Future<CareQueue> loadForAccount(String accountId) async {
    return _queue ?? CareQueue.empty(accountId);
  }

  @override
  Future<void> save(CareQueue queue) async {
    _queue = queue;
  }
}

PetSummary _demoParent(String id, String name, int level) {
  return PetSummary(
    id: id,
    ownerId: 'demo-player-1',
    genome: const {'element': 'Water'},
    name: name,
    stage: 'adult',
    level: level,
    xp: 12400,
    needs: const PetNeeds(hunger: 88, energy: 90, hygiene: 84, mood: 91),
    stats: const PetStats(strength: 21, agility: 19, endurance: 23, focus: 18),
    generation: 0,
    isActive: false,
    createdAt: DateTime.utc(2026, 5, 2),
    isWeak: false,
    careRevision: 40,
    needsUpdatedAt: DateTime.utc(2026, 7, 25, 12),
  );
}

TechniqueCardSummary _demoCard(
  String id,
  TechniqueType type,
  TechniqueRarity rarity,
  double baseDamage,
) {
  return TechniqueCardSummary(
    id: id,
    ownerId: 'demo-player-1',
    type: type,
    element: CreatureElement.earth,
    rarity: rarity,
    baseDamage: baseDamage,
    speed: 52,
    staminaCost: 14,
    critChance: 0.08,
    effect: TechniqueEffect.none,
    effectValue: 0,
    quality: 48,
    createdAt: DateTime.utc(2026, 7, 24),
  );
}

final _demoCards = List<TechniqueCardSummary>.unmodifiable([
  _demoCard('demo-card-1', TechniqueType.jab, TechniqueRarity.common, 12),
  _demoCard('demo-card-2', TechniqueType.hook, TechniqueRarity.uncommon, 18),
  _demoCard('demo-card-3', TechniqueType.uppercut, TechniqueRarity.rare, 24),
  _demoCard('demo-card-4', TechniqueType.cross, TechniqueRarity.common, 15),
  _demoCard('demo-card-5', TechniqueType.kick, TechniqueRarity.uncommon, 20),
  _demoCard('demo-card-6', TechniqueType.elbow, TechniqueRarity.epic, 29),
]);

DailyActivity _demoDay(String date, int steps, int vitality) {
  return DailyActivity(
    date: date,
    snapshot: ActivitySnapshotSummary(
      steps: steps,
      sleepMinutes: 431,
      activeCalories: 380,
      workouts: steps > 9000 ? 1 : 0,
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
    updatedAt: DateTime.utc(2026, 7, 25, 20),
  );
}

final _demoWeek = List<DailyActivity>.unmodifiable([
  _demoDay('2026-07-25', 11240, 118),
  _demoDay('2026-07-24', 8130, 86),
  _demoDay('2026-07-23', 6420, 61),
]);

const _demoCatalog = ShopCatalog(
  items: [
    ShopCatalogItem(
      id: ShopItemId.apple,
      category: ShopCategory.care,
      unitPrice: 20,
      isStackable: true,
    ),
    ShopCatalogItem(
      id: ShopItemId.steak,
      category: ShopCategory.care,
      unitPrice: 80,
      isStackable: true,
    ),
    ShopCatalogItem(
      id: ShopItemId.energyDrink,
      category: ShopCategory.care,
      unitPrice: 50,
      isStackable: true,
    ),
    ShopCatalogItem(
      id: ShopItemId.soap,
      category: ShopCategory.care,
      unitPrice: 30,
      isStackable: true,
    ),
    ShopCatalogItem(
      id: ShopItemId.shampoo,
      category: ShopCategory.care,
      unitPrice: 60,
      isStackable: true,
    ),
    ShopCatalogItem(
      id: ShopItemId.loveCrystal,
      category: ShopCategory.breeding,
      unitPrice: 200,
      isStackable: true,
    ),
  ],
);
