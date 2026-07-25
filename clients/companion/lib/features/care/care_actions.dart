import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../app/theme.dart';
import '../../core/models/care_models.dart';
import '../../core/models/profile_models.dart';
import '../../core/network/gochya_api_client.dart';
import 'care_repository.dart';

class CareActions extends ConsumerStatefulWidget {
  const CareActions({
    required this.accessToken,
    required this.pet,
    required this.onSnapshotChanged,
    super.key,
  });

  final String accessToken;
  final PetSummary pet;
  final VoidCallback onSnapshotChanged;

  @override
  ConsumerState<CareActions> createState() => _CareActionsState();
}

class _CareActionsState extends ConsumerState<CareActions> {
  CareIntent? _pendingIntent;
  var _isSubmitting = false;
  Object? _error;

  @override
  void didUpdateWidget(covariant CareActions oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.pet.id != widget.pet.id ||
        oldWidget.pet.careRevision != widget.pet.careRevision) {
      _pendingIntent = null;
      _error = null;
    }
  }

  @override
  Widget build(BuildContext context) {
    final sleepingUntil = widget.pet.sleepingUntil;
    final isSleeping =
        sleepingUntil != null && sleepingUntil.isAfter(DateTime.now());
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              'Забота',
              style: Theme.of(
                context,
              ).textTheme.titleLarge?.copyWith(fontWeight: FontWeight.w800),
            ),
            const SizedBox(height: 6),
            Text(
              isSleeping
                  ? '${widget.pet.label} спит до ${_timeLabel(sleepingUntil)}.'
                  : 'Результат каждого действия подтверждает сервер.',
              style: const TextStyle(color: GochyaColors.muted),
            ),
            const SizedBox(height: 16),
            Wrap(
              spacing: 10,
              runSpacing: 10,
              children: [
                _CareButton(
                  key: const Key('care-feed'),
                  label: 'Яблоко',
                  icon: Icons.restaurant_rounded,
                  isBusy: _isBusy(CareOperation.feed),
                  onPressed: _canRun(CareOperation.feed, itemId: 'apple')
                      ? () => _execute(CareOperation.feed, itemId: 'apple')
                      : null,
                ),
                _CareButton(
                  key: const Key('care-clean'),
                  label: 'Почистить',
                  icon: Icons.auto_awesome_rounded,
                  isBusy: _isBusy(CareOperation.clean),
                  onPressed: _canRun(CareOperation.clean)
                      ? () => _execute(CareOperation.clean)
                      : null,
                ),
                _CareButton(
                  key: const Key('care-play'),
                  label: 'Поиграть',
                  icon: Icons.sports_esports_rounded,
                  isBusy: _isBusy(CareOperation.play),
                  onPressed: _canRun(CareOperation.play)
                      ? () => _execute(CareOperation.play)
                      : null,
                ),
                _CareButton(
                  key: const Key('care-sleep'),
                  label: isSleeping ? 'Уже спит' : 'Уложить спать',
                  icon: Icons.bedtime_rounded,
                  isBusy: _isBusy(CareOperation.sleep),
                  onPressed: isSleeping || !_canRun(CareOperation.sleep)
                      ? null
                      : () => _execute(CareOperation.sleep),
                ),
              ],
            ),
            if (_error != null) ...[
              const SizedBox(height: 14),
              Text(
                _careRequestError(_error!),
                key: const Key('care-error'),
                style: const TextStyle(color: GochyaColors.warning),
              ),
              if (_pendingIntent != null) ...[
                const SizedBox(height: 8),
                TextButton.icon(
                  key: const Key('care-retry'),
                  onPressed: _isSubmitting
                      ? null
                      : () => _execute(
                          _pendingIntent!.operation,
                          itemId: _pendingIntent!.itemId,
                        ),
                  icon: const Icon(Icons.refresh_rounded),
                  label: const Text('Повторить то же действие'),
                ),
              ],
            ],
          ],
        ),
      ),
    );
  }

  bool _isBusy(CareOperation operation) {
    return _isSubmitting && _pendingIntent?.operation == operation;
  }

  bool _canRun(CareOperation operation, {String? itemId}) {
    if (_isSubmitting) {
      return false;
    }
    final pending = _pendingIntent;
    return pending == null ||
        (pending.operation == operation && pending.itemId == itemId);
  }

  Future<void> _execute(CareOperation operation, {String? itemId}) async {
    final pending = _pendingIntent;
    if (pending != null &&
        (pending.operation != operation || pending.itemId != itemId)) {
      return;
    }
    final intent = pending ?? newCareIntent(operation, itemId: itemId);
    setState(() {
      _pendingIntent = intent;
      _isSubmitting = true;
      _error = null;
    });

    try {
      final result = await ref
          .read(careRepositoryProvider)
          .execute(
            accessToken: widget.accessToken,
            petId: widget.pet.id,
            baseRevision: widget.pet.careRevision,
            intent: intent,
          );
      if (!mounted) {
        return;
      }
      if (result.status == CareCommandStatus.retryable) {
        setState(() {
          _isSubmitting = false;
          _error = const _RetryableCareError();
        });
        return;
      }
      setState(() {
        _pendingIntent = null;
        _isSubmitting = false;
      });
      final message = result.status.isApplied
          ? _successMessage(operation, result.status)
          : _rejectionMessage(result);
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text(message)));
      widget.onSnapshotChanged();
    } catch (error) {
      if (!mounted) {
        return;
      }
      setState(() {
        _isSubmitting = false;
        _error = error;
      });
      if (error is ApiException && error.isUnauthorized) {
        widget.onSnapshotChanged();
      }
    }
  }
}

class _CareButton extends StatelessWidget {
  const _CareButton({
    required this.label,
    required this.icon,
    required this.isBusy,
    required this.onPressed,
    super.key,
  });

  final String label;
  final IconData icon;
  final bool isBusy;
  final VoidCallback? onPressed;

  @override
  Widget build(BuildContext context) {
    return FilledButton.tonalIcon(
      onPressed: onPressed,
      icon: isBusy
          ? const SizedBox.square(
              dimension: 18,
              child: CircularProgressIndicator(strokeWidth: 2),
            )
          : Icon(icon),
      label: Text(label),
    );
  }
}

class _RetryableCareError implements Exception {
  const _RetryableCareError();
}

String _timeLabel(DateTime value) {
  final local = value.toLocal();
  return '${local.hour.toString().padLeft(2, '0')}:'
      '${local.minute.toString().padLeft(2, '0')}';
}

String _successMessage(CareOperation operation, CareCommandStatus status) {
  if (status == CareCommandStatus.alreadyApplied) {
    return 'Действие уже было принято — состояние синхронизировано.';
  }
  return switch (operation) {
    CareOperation.feed => 'Питомец накормлен.',
    CareOperation.clean => 'Питомец снова чистый.',
    CareOperation.play => 'Вы отлично поиграли.',
    CareOperation.sleep => 'Питомец уснул.',
  };
}

String _rejectionMessage(CareCommandResult result) {
  return switch (result.errorCode) {
    'revision_conflict' =>
      'Состояние изменилось на другом устройстве. Данные обновлены.',
    'already_sleeping' => 'Питомец уже спит. Данные обновлены.',
    'item_unavailable' => 'Нужного предмета больше нет в инвентаре.',
    'command_expired' => 'Действие устарело и не было применено.',
    'idempotency_conflict' =>
      'Сервер отклонил конфликтующий идентификатор действия.',
    _ => 'Действие не было применено. Состояние обновлено.',
  };
}

String _careRequestError(Object error) {
  if (error is _RetryableCareError) {
    return 'Сервер попросил повторить действие с тем же идентификатором.';
  }
  if (error is ApiException && error.isUnauthorized) {
    return 'Сессия истекла. Обновляем экран входа…';
  }
  if (error is ApiException && error.code == 'invalid_response') {
    return 'Сервер вернул повреждённый ответ. Действие можно безопасно повторить.';
  }
  return 'Не удалось подтвердить действие. Повтор использует тот же '
      'идентификатор и не создаст дубль.';
}
