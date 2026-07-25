import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gochya_companion/core/models/care_models.dart';
import 'package:gochya_companion/core/models/profile_models.dart';
import 'package:gochya_companion/core/network/gochya_api_client.dart';
import 'package:gochya_companion/features/care/care_actions.dart';
import 'package:gochya_companion/features/care/care_repository.dart';

void main() {
  testWidgets('submits server-authoritative care and refreshes snapshot', (
    tester,
  ) async {
    final repository = _RecordingCareRepository();
    var refreshes = 0;
    await _pumpCare(
      tester,
      repository: repository,
      onSnapshotChanged: () => refreshes++,
    );

    await tester.tap(find.byKey(const Key('care-feed')));
    await tester.pumpAndSettle();

    expect(repository.intents, hasLength(1));
    expect(repository.intents.single.operation, CareOperation.feed);
    expect(repository.intents.single.itemId, 'apple');
    expect(repository.baseRevisions.single, 9);
    expect(refreshes, 1);
    expect(find.text('Питомец накормлен.'), findsOneWidget);
  });

  testWidgets('network retry preserves the complete care intent', (
    tester,
  ) async {
    final repository = _RecordingCareRepository(failFirst: true);
    var refreshes = 0;
    await _pumpCare(
      tester,
      repository: repository,
      onSnapshotChanged: () => refreshes++,
    );

    await tester.tap(find.byKey(const Key('care-play')));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('care-error')), findsOneWidget);
    expect(find.byKey(const Key('care-retry')), findsOneWidget);
    final first = repository.intents.single;

    await tester.tap(find.byKey(const Key('care-retry')));
    await tester.pumpAndSettle();

    expect(repository.intents, hasLength(2));
    final retry = repository.intents.last;
    expect(retry.operationId, first.operationId);
    expect(retry.operation, first.operation);
    expect(retry.itemId, first.itemId);
    expect(retry.clientWallTime, first.clientWallTime);
    expect(retry.clientMonotonicOffsetMs, first.clientMonotonicOffsetMs);
    expect(refreshes, 1);
  });
}

Future<void> _pumpCare(
  WidgetTester tester, {
  required CareRepository repository,
  required VoidCallback onSnapshotChanged,
}) {
  return tester.pumpWidget(
    ProviderScope(
      overrides: [careRepositoryProvider.overrideWithValue(repository)],
      child: MaterialApp(
        home: Scaffold(
          body: SingleChildScrollView(
            child: CareActions(
              accessToken: 'access-token',
              pet: _pet,
              onSnapshotChanged: onSnapshotChanged,
            ),
          ),
        ),
      ),
    ),
  );
}

class _RecordingCareRepository implements CareRepository {
  _RecordingCareRepository({this.failFirst = false});

  final bool failFirst;
  final List<CareIntent> intents = [];
  final List<int> baseRevisions = [];

  @override
  Future<CareCommandResult> execute({
    required String accessToken,
    required String petId,
    required int baseRevision,
    required CareIntent intent,
  }) async {
    intents.add(intent);
    baseRevisions.add(baseRevision);
    if (failFirst && intents.length == 1) {
      throw const ApiException(code: 'network_error', message: 'offline');
    }
    return CareCommandResult(
      operationId: intent.operationId,
      status: CareCommandStatus.applied,
      snapshot: CarePetSnapshot(
        id: petId,
        needs: const PetNeeds(hunger: 91, energy: 72, hygiene: 65, mood: 94),
        revision: baseRevision + 1,
        isWeak: false,
        needsUpdatedAt: DateTime.utc(2026, 7, 25),
      ),
    );
  }
}

final _pet = PetSummary(
  id: '33333333-3333-4333-8333-333333333333',
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
  isWeak: false,
  careRevision: 9,
  needsUpdatedAt: DateTime.utc(2026, 7, 24),
);
