import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../app/theme.dart';
import '../../core/models/profile_models.dart';
import '../../core/network/gochya_api_client.dart';
import 'profile_repository.dart';

class LineageScreen extends ConsumerWidget {
  const LineageScreen({
    required this.accessToken,
    required this.pet,
    super.key,
  });

  final String accessToken;
  final PetSummary pet;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final request = (accessToken: accessToken, petId: pet.id);
    final lineage = ref.watch(lineageProvider(request));
    return Scaffold(
      appBar: AppBar(title: Text('Родословная · ${pet.label}')),
      body: lineage.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (error, stackTrace) => _LineageError(
          error: error,
          onRetry: () => ref.invalidate(lineageProvider(request)),
        ),
        data: (tree) => RefreshIndicator(
          onRefresh: () => ref.refresh(lineageProvider(request).future),
          child: _LineageContent(tree: tree),
        ),
      ),
    );
  }
}

class _LineageContent extends StatelessWidget {
  const _LineageContent({required this.tree});

  final LineageTree tree;

  @override
  Widget build(BuildContext context) {
    return ListView(
      physics: const AlwaysScrollableScrollPhysics(),
      padding: const EdgeInsets.fromLTRB(20, 8, 20, 28),
      children: [
        if (tree.truncated)
          const Card(
            child: Padding(
              padding: EdgeInsets.all(16),
              child: Row(
                children: [
                  Icon(Icons.more_horiz_rounded, color: GochyaColors.secondary),
                  SizedBox(width: 12),
                  Expanded(
                    child: Text(
                      'У рода есть более глубокие ветви. Сервер показывает '
                      'не более трёх поколений.',
                    ),
                  ),
                ],
              ),
            ),
          ),
        if (tree.truncated) const SizedBox(height: 12),
        for (var depth = 0; depth <= tree.maxDepth; depth++) ...[
          if (tree.nodes.any((node) => node.ancestorDepth == depth)) ...[
            Padding(
              padding: const EdgeInsets.fromLTRB(4, 14, 4, 8),
              child: Text(
                _depthLabel(depth),
                style: Theme.of(context).textTheme.titleMedium?.copyWith(
                  color: GochyaColors.muted,
                  fontWeight: FontWeight.w700,
                ),
              ),
            ),
            for (final node in tree.nodes.where(
              (node) => node.ancestorDepth == depth,
            ))
              Padding(
                padding: const EdgeInsets.only(bottom: 10),
                child: _LineageNodeCard(node: node),
              ),
          ],
        ],
      ],
    );
  }
}

class _LineageNodeCard extends StatelessWidget {
  const _LineageNodeCard({required this.node});

  final LineageNode node;

  @override
  Widget build(BuildContext context) {
    return Card(
      child: ListTile(
        contentPadding: const EdgeInsets.symmetric(horizontal: 18, vertical: 8),
        leading: CircleAvatar(
          backgroundColor: GochyaColors.primary.withValues(alpha: 0.22),
          child: const Icon(Icons.pets_rounded),
        ),
        title: Text(
          node.label,
          maxLines: 1,
          overflow: TextOverflow.ellipsis,
          style: const TextStyle(fontWeight: FontWeight.w800),
        ),
        subtitle: Text(
          '${node.stage} · ур. ${node.level} · поколение ${node.generation}',
        ),
        trailing: node.mutatedGenes == 0
            ? null
            : Tooltip(
                message:
                    'Mutation mask: 0x${node.mutatedGenes.toRadixString(16)}',
                child: const Icon(
                  Icons.auto_awesome_rounded,
                  color: GochyaColors.secondary,
                ),
              ),
      ),
    );
  }
}

class _LineageError extends StatelessWidget {
  const _LineageError({required this.error, required this.onRetry});

  final Object error;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    final requestId = error is ApiException
        ? (error as ApiException).requestId
        : null;
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(28),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.account_tree_outlined, size: 64),
            const SizedBox(height: 16),
            const Text(
              'Не удалось загрузить родословную',
              textAlign: TextAlign.center,
              style: TextStyle(fontSize: 20, fontWeight: FontWeight.w800),
            ),
            if (requestId != null) ...[
              const SizedBox(height: 8),
              Text('Request ID: $requestId'),
            ],
            const SizedBox(height: 20),
            FilledButton.icon(
              onPressed: onRetry,
              icon: const Icon(Icons.refresh_rounded),
              label: const Text('Повторить'),
            ),
          ],
        ),
      ),
    );
  }
}

String _depthLabel(int depth) {
  return switch (depth) {
    0 => 'Питомец',
    1 => 'Родители',
    2 => 'Бабушки и дедушки',
    3 => 'Прабабушки и прадедушки',
    _ => 'Поколение $depth',
  };
}
