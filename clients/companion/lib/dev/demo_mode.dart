import 'dart:math';

import 'package:flutter/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../core/models/care_models.dart';
import '../core/models/profile_models.dart';
import '../core/models/shop_models.dart';
import '../core/network/gochya_api_client.dart';
import '../core/session/session_store.dart';
import '../features/auth/auth_repository.dart';
import '../features/care/care_queue_store.dart';
import '../features/care/care_repository.dart';
import '../features/home/profile_repository.dart';
import '../features/session/session_controller.dart';
import '../features/shop/shop_repository.dart';

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
    return HomeSnapshot(profile: _DemoPlayerState.profile, pets: [state.pet]);
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
