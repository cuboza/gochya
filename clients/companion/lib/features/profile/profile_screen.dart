import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../app/gochya_loader.dart';
import '../../app/theme.dart';
import '../../core/models/profile_models.dart';
import '../creatures/creature_art.dart';
import '../home/profile_repository.dart';
import '../session/session_controller.dart';

/// The player and their creatures.
///
/// `UX_UI.md` §7.5 also asks for league, rating, friends and a battle pass.
/// None of those have a server endpoint, so none of them are drawn here: an
/// empty "Лига —" row would be a promise the backend cannot keep. What is
/// shown comes entirely from `/v1/me` and `/v1/me/pets`.
class ProfileScreen extends ConsumerWidget {
  const ProfileScreen({required this.accessToken, super.key});

  final String accessToken;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final snapshot = ref.watch(homeSnapshotProvider(accessToken));
    return Scaffold(
      appBar: AppBar(title: const Text('Профиль')),
      body: snapshot.when(
        skipLoadingOnReload: true,
        loading: () => const GochyaLoader(caption: 'Открываем профиль…'),
        error: (error, stackTrace) => _ProfileError(
          onRetry: () => ref.invalidate(homeSnapshotProvider(accessToken)),
          onSignOut: () =>
              ref.read(sessionControllerProvider.notifier).signOut(),
        ),
        data: (value) => RefreshIndicator(
          onRefresh: () =>
              ref.refresh(homeSnapshotProvider(accessToken).future),
          child: ListView(
            physics: const AlwaysScrollableScrollPhysics(),
            padding: const EdgeInsets.fromLTRB(20, 12, 20, 28),
            children: [
              _PlayerCard(profile: value.profile),
              const SizedBox(height: 16),
              Text(
                'Питомцы · ${value.pets.length}',
                style: Theme.of(
                  context,
                ).textTheme.titleLarge?.copyWith(fontWeight: FontWeight.w800),
              ),
              const SizedBox(height: 12),
              if (value.pets.isEmpty)
                const _NoPetsCard()
              else
                for (final pet in value.pets) ...[
                  _PetRow(pet: pet, isActive: pet.id == value.activePet?.id),
                  const SizedBox(height: 10),
                ],
              const SizedBox(height: 16),
              _SettingsCard(
                onSignOut: () =>
                    ref.read(sessionControllerProvider.notifier).signOut(),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _PlayerCard extends StatelessWidget {
  const _PlayerCard({required this.profile});

  final PlayerProfile profile;

  @override
  Widget build(BuildContext context) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              profile.label,
              style: Theme.of(
                context,
              ).textTheme.headlineSmall?.copyWith(fontWeight: FontWeight.w800),
            ),
            const SizedBox(height: 2),
            Text(
              '@${profile.username}',
              style: Theme.of(
                context,
              ).textTheme.bodyMedium?.copyWith(color: GochyaColors.muted),
            ),
            const SizedBox(height: 16),
            Row(
              children: [
                Expanded(
                  child: _Stat(
                    value: '${profile.streakDays}',
                    caption: 'дней подряд',
                    color: GochyaColors.secondary,
                  ),
                ),
                Expanded(
                  child: _Stat(
                    value: _since(profile.createdAt),
                    caption: 'в игре с',
                    color: GochyaColors.primary,
                  ),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }

  static String _since(DateTime created) {
    final local = created.toLocal();
    return '${local.day.toString().padLeft(2, '0')}.'
        '${local.month.toString().padLeft(2, '0')}.${local.year}';
  }
}

class _Stat extends StatelessWidget {
  const _Stat({
    required this.value,
    required this.caption,
    required this.color,
  });

  final String value;
  final String caption;
  final Color color;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          value,
          style: Theme.of(context).textTheme.titleLarge?.copyWith(
            fontWeight: FontWeight.w800,
            color: color,
          ),
        ),
        Text(
          caption,
          style: Theme.of(
            context,
          ).textTheme.bodySmall?.copyWith(color: GochyaColors.muted),
        ),
      ],
    );
  }
}

class _PetRow extends StatelessWidget {
  const _PetRow({required this.pet, required this.isActive});

  final PetSummary pet;
  final bool isActive;

  @override
  Widget build(BuildContext context) {
    final element = creatureElementOf(pet.genome);
    final tint = element?.tint ?? GochyaColors.primary;
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Row(
          children: [
            Container(
              width: 44,
              height: 44,
              decoration: BoxDecoration(
                color: tint.withValues(alpha: 0.22),
                shape: BoxShape.circle,
                border: Border.all(color: tint, width: 1.5),
              ),
              child: Icon(Icons.pets, color: tint, size: 22),
            ),
            const SizedBox(width: 14),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Row(
                    children: [
                      Flexible(
                        child: Text(
                          pet.label,
                          overflow: TextOverflow.ellipsis,
                          style: Theme.of(context).textTheme.titleMedium
                              ?.copyWith(fontWeight: FontWeight.w800),
                        ),
                      ),
                      if (isActive) ...[
                        const SizedBox(width: 8),
                        const _ActiveChip(),
                      ],
                    ],
                  ),
                  const SizedBox(height: 2),
                  Text(
                    element == null
                        ? '${pet.stageLabel} · уровень ${pet.level}'
                        : '${element.label} · ${pet.stageLabel} · '
                              'уровень ${pet.level}',
                    style: Theme.of(
                      context,
                    ).textTheme.bodySmall?.copyWith(color: GochyaColors.muted),
                  ),
                  Text(
                    'поколение ${pet.generation}',
                    style: Theme.of(
                      context,
                    ).textTheme.bodySmall?.copyWith(color: GochyaColors.muted),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _ActiveChip extends StatelessWidget {
  const _ActiveChip();

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
      decoration: BoxDecoration(
        color: GochyaColors.success.withValues(alpha: 0.18),
        borderRadius: BorderRadius.circular(999),
      ),
      child: Text(
        'активный',
        style: Theme.of(context).textTheme.labelSmall?.copyWith(
          color: GochyaColors.success,
          fontWeight: FontWeight.w700,
        ),
      ),
    );
  }
}

class _NoPetsCard extends StatelessWidget {
  const _NoPetsCard();

  @override
  Widget build(BuildContext context) {
    return const Card(
      child: Padding(
        padding: EdgeInsets.all(20),
        child: Text('Питомцев пока нет. Вылупи первого на главном экране.'),
      ),
    );
  }
}

class _SettingsCard extends StatelessWidget {
  const _SettingsCard({required this.onSignOut});

  final VoidCallback onSignOut;

  @override
  Widget build(BuildContext context) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              'Настройки',
              style: Theme.of(
                context,
              ).textTheme.titleLarge?.copyWith(fontWeight: FontWeight.w800),
            ),
            const SizedBox(height: 12),
            FilledButton.tonalIcon(
              onPressed: onSignOut,
              icon: const Icon(Icons.logout_rounded),
              label: const Text('Выйти и отозвать сессию'),
            ),
          ],
        ),
      ),
    );
  }
}

class _ProfileError extends StatelessWidget {
  const _ProfileError({required this.onRetry, required this.onSignOut});

  final VoidCallback onRetry;
  final VoidCallback onSignOut;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Text(
              'Не удалось прочитать профиль. Проверьте соединение.',
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 16),
            FilledButton(onPressed: onRetry, child: const Text('Повторить')),
            const SizedBox(height: 8),
            TextButton.icon(
              onPressed: onSignOut,
              icon: const Icon(Icons.logout_rounded),
              label: const Text('Выйти'),
            ),
          ],
        ),
      ),
    );
  }
}
