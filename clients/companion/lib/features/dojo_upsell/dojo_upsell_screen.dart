import 'package:flutter/material.dart';

import '../../app/theme.dart';
import '../../core/models/technique_models.dart';
import '../techniques/technique_content.dart';

/// Explains what recording a punch adds, per `CLIENT_COMPANION.md` §5а.
///
/// The tone is fixed by the spec and by `MECHANIC_COMBAT_RECORDING.md` §0:
/// recording buys *quality and identity*, never a monopoly on strength. A phone
/// player is already competitive, so nothing here may read as "buy a watch or
/// lose" — no countdown, no locked button, no greyed-out feature list.
///
/// There is deliberately no "connect" button: watch pairing does not exist in
/// this client yet, and a button that does nothing would be worse than a
/// sentence that tells the truth.
class DojoUpsellScreen extends StatelessWidget {
  const DojoUpsellScreen({super.key});

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Запись приёмов')),
      body: ListView(
        padding: const EdgeInsets.fromLTRB(20, 12, 20, 28),
        children: [
          Text(
            'Dojo живёт на часах',
            style: Theme.of(
              context,
            ).textTheme.headlineSmall?.copyWith(fontWeight: FontWeight.w800),
          ),
          const SizedBox(height: 8),
          Text(
            'Чтобы распознать удар, нужен акселерометр на запястье и пульс. '
            'Телефон в кармане этого не видит, поэтому запись есть только на '
            'часах.',
            style: Theme.of(
              context,
            ).textTheme.bodyLarge?.copyWith(color: GochyaColors.muted),
          ),
          const SizedBox(height: 24),
          const _PathCard(
            title: 'Игровая добыча',
            where: 'телефон и часы',
            ceiling: TechniqueRarity.epic,
            points: ['Задания и награды PvP', 'Дневная активность', 'Гача'],
          ),
          const SizedBox(height: 14),
          const _PathCard(
            title: 'Запись удара',
            where: 'только часы',
            ceiling: TechniqueRarity.mythic,
            points: [
              'Карты твоего стиля',
              'Spirit-бонус',
              'Signature-приём — питомец дерётся твоим ударом',
            ],
          ),
          const SizedBox(height: 24),
          const _CompetitiveCard(),
          const SizedBox(height: 14),
          const _ConnectCard(),
        ],
      ),
    );
  }
}

class _PathCard extends StatelessWidget {
  const _PathCard({
    required this.title,
    required this.where,
    required this.ceiling,
    required this.points,
  });

  final String title;
  final String where;
  final TechniqueRarity ceiling;
  final List<String> points;

  @override
  Widget build(BuildContext context) {
    final frame = ceiling.frameColor;
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Expanded(
                  child: Text(
                    title,
                    style: Theme.of(context).textTheme.titleLarge?.copyWith(
                      fontWeight: FontWeight.w800,
                    ),
                  ),
                ),
                Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 12,
                    vertical: 5,
                  ),
                  decoration: BoxDecoration(
                    border: Border.all(color: frame, width: 1.5),
                    borderRadius: BorderRadius.circular(999),
                  ),
                  child: Text(
                    'до ${ceiling.label}',
                    style: Theme.of(context).textTheme.labelMedium?.copyWith(
                      color: frame,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 4),
            Text(
              where,
              style: Theme.of(
                context,
              ).textTheme.bodyMedium?.copyWith(color: GochyaColors.muted),
            ),
            const SizedBox(height: 14),
            for (final point in points)
              Padding(
                padding: const EdgeInsets.only(bottom: 8),
                child: Row(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Icon(Icons.circle, size: 6, color: frame),
                    const SizedBox(width: 10),
                    Expanded(child: Text(point)),
                  ],
                ),
              ),
          ],
        ),
      ),
    );
  }
}

class _CompetitiveCard extends StatelessWidget {
  const _CompetitiveCard();

  @override
  Widget build(BuildContext context) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                const Icon(
                  Icons.verified_outlined,
                  color: GochyaColors.success,
                  size: 22,
                ),
                const SizedBox(width: 10),
                Expanded(
                  child: Text(
                    'Ты уже конкурентоспособен',
                    style: Theme.of(context).textTheme.titleLarge?.copyWith(
                      fontWeight: FontWeight.w800,
                    ),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 12),
            const Text(
              'Силу боя решает не редкость сама по себе, а статы питомца, '
              'карты и снаряжение вместе. Карт Epic из игровой добычи, статов '
              'от бридинга и снаряжения хватает для любой лиги, а матчмейкинг '
              'сводит равных по силе.',
            ),
            const SizedBox(height: 12),
            Text(
              'Записанная карта даёт твой стиль и чуть больше эффективности за '
              'то же усилие — но не пропуск наверх. Signature-приём остаётся '
              'эксклюзивом часов, и это про идентичность, а не про превосходство.',
              style: Theme.of(
                context,
              ).textTheme.bodyMedium?.copyWith(color: GochyaColors.muted),
            ),
          ],
        ),
      ),
    );
  }
}

class _ConnectCard extends StatelessWidget {
  const _ConnectCard();

  @override
  Widget build(BuildContext context) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                const Icon(
                  Icons.watch_outlined,
                  color: GochyaColors.primary,
                  size: 22,
                ),
                const SizedBox(width: 10),
                Expanded(
                  child: Text(
                    'Есть Galaxy Watch или Apple Watch?',
                    style: Theme.of(context).textTheme.titleMedium?.copyWith(
                      fontWeight: FontWeight.w800,
                    ),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 10),
            const Text(
              'Установи GOCHYA на часы тем же аккаунтом — питомец там тот же, '
              'и Dojo откроется сам. Ничего доплачивать и ничего ждать не нужно.',
            ),
          ],
        ),
      ),
    );
  }
}
