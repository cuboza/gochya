import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../app/theme.dart';
import '../../core/models/onboarding_models.dart';
import '../../core/network/gochya_api_client.dart';
import 'onboarding_repository.dart';

class OnboardingScreen extends ConsumerStatefulWidget {
  const OnboardingScreen({required this.accessToken, super.key});

  final String accessToken;

  @override
  ConsumerState<OnboardingScreen> createState() => _OnboardingScreenState();
}

class _OnboardingScreenState extends ConsumerState<OnboardingScreen> {
  DateTime? _birthDate;
  StarterElement? _element;
  var _ageAccepted = false;
  var _parentalConsentRequired = false;
  var _isSubmitting = false;
  Object? _error;
  String? _ageIdempotencyKey;
  String? _starterIdempotencyKey;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Первый питомец')),
      body: SafeArea(
        child: ListView(
          padding: const EdgeInsets.fromLTRB(20, 8, 20, 32),
          children: [
            _ProgressHeader(step: _ageAccepted ? 2 : 1),
            const SizedBox(height: 20),
            if (_parentalConsentRequired)
              const _ParentalConsentState()
            else if (_ageAccepted)
              _buildStarterStep()
            else
              _buildAgeStep(),
            if (_error != null) ...[
              const SizedBox(height: 16),
              _SubmissionError(error: _error!),
            ],
          ],
        ),
      ),
    );
  }

  Widget _buildAgeStep() {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            const Icon(
              Icons.cake_outlined,
              size: 52,
              color: GochyaColors.secondary,
            ),
            const SizedBox(height: 16),
            Text(
              'Сколько тебе лет?',
              textAlign: TextAlign.center,
              style: Theme.of(
                context,
              ).textTheme.headlineSmall?.copyWith(fontWeight: FontWeight.w800),
            ),
            const SizedBox(height: 8),
            const Text(
              'Дата нужна один раз для возрастной категории. Сервер не '
              'сохраняет ни дату, ни её хеш.',
              textAlign: TextAlign.center,
              style: TextStyle(color: GochyaColors.muted),
            ),
            const SizedBox(height: 20),
            OutlinedButton.icon(
              onPressed: _isSubmitting ? null : _chooseBirthDate,
              icon: const Icon(Icons.calendar_month_outlined),
              label: Text(
                _birthDate == null
                    ? 'Выбрать дату рождения'
                    : _formatDisplayDate(_birthDate!),
              ),
            ),
            const SizedBox(height: 12),
            FilledButton(
              onPressed: _birthDate == null || _isSubmitting
                  ? null
                  : _submitAgeGate,
              child: _isSubmitting
                  ? const _ButtonProgress(label: 'Проверяем')
                  : const Text('Продолжить'),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildStarterStep() {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text(
              'Выбери стихию яйца',
              textAlign: TextAlign.center,
              style: Theme.of(
                context,
              ).textTheme.headlineSmall?.copyWith(fontWeight: FontWeight.w800),
            ),
            const SizedBox(height: 8),
            const Text(
              'Сервер создаст геном питомца в Shared Core. Выбор нельзя '
              'заменить после создания яйца.',
              textAlign: TextAlign.center,
              style: TextStyle(color: GochyaColors.muted),
            ),
            const SizedBox(height: 20),
            for (final element in StarterElement.values) ...[
              _ElementChoice(
                element: element,
                selected: _element == element,
                onSelected: _isSubmitting
                    ? null
                    : () {
                        setState(() {
                          if (_element != element) {
                            _starterIdempotencyKey = null;
                          }
                          _element = element;
                          _error = null;
                        });
                      },
              ),
              if (element != StarterElement.values.last)
                const SizedBox(height: 10),
            ],
            const SizedBox(height: 20),
            FilledButton(
              onPressed: _element == null || _isSubmitting
                  ? null
                  : _submitStarterEgg,
              child: _isSubmitting
                  ? const _ButtonProgress(label: 'Создаём яйцо')
                  : const Text('Выбрать яйцо'),
            ),
          ],
        ),
      ),
    );
  }

  Future<void> _chooseBirthDate() async {
    final now = DateTime.now();
    final selected = await showDatePicker(
      context: context,
      initialDate: _birthDate ?? DateTime(now.year - 18, now.month, now.day),
      firstDate: DateTime(now.year - 120, now.month, now.day),
      lastDate: DateTime(now.year, now.month, now.day),
      helpText: 'Дата рождения',
      cancelText: 'Отмена',
      confirmText: 'Выбрать',
    );
    if (selected == null || !mounted) {
      return;
    }
    setState(() {
      if (_birthDate != selected) {
        _ageIdempotencyKey = null;
      }
      _birthDate = selected;
      _error = null;
    });
  }

  Future<void> _submitAgeGate() async {
    final birthDate = _birthDate;
    if (birthDate == null) {
      return;
    }
    setState(() {
      _isSubmitting = true;
      _error = null;
      _ageIdempotencyKey ??= newIdempotencyKey();
    });
    try {
      final result = await ref
          .read(onboardingRepositoryProvider)
          .recordAgeGate(
            accessToken: widget.accessToken,
            birthDate: birthDate,
            idempotencyKey: _ageIdempotencyKey!,
          );
      if (!mounted) {
        return;
      }
      setState(() {
        _isSubmitting = false;
        _ageAccepted = result.isEligible;
        _parentalConsentRequired = !result.isEligible;
      });
    } catch (error) {
      if (!mounted) {
        return;
      }
      setState(() {
        _isSubmitting = false;
        _error = error;
      });
    }
  }

  Future<void> _submitStarterEgg() async {
    final element = _element;
    if (element == null) {
      return;
    }
    setState(() {
      _isSubmitting = true;
      _error = null;
      _starterIdempotencyKey ??= newIdempotencyKey();
    });
    try {
      final result = await ref
          .read(onboardingRepositoryProvider)
          .selectStarterEgg(
            accessToken: widget.accessToken,
            element: element,
            idempotencyKey: _starterIdempotencyKey!,
          );
      if (!mounted) {
        return;
      }
      Navigator.of(context).pop(result);
    } catch (error) {
      if (!mounted) {
        return;
      }
      setState(() {
        _isSubmitting = false;
        _error = error;
      });
    }
  }
}

class _ProgressHeader extends StatelessWidget {
  const _ProgressHeader({required this.step});

  final int step;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          'Шаг $step из 2',
          style: const TextStyle(
            color: GochyaColors.muted,
            fontWeight: FontWeight.w700,
          ),
        ),
        const SizedBox(height: 8),
        LinearProgressIndicator(
          value: step / 2,
          minHeight: 8,
          borderRadius: BorderRadius.circular(8),
        ),
      ],
    );
  }
}

class _ElementChoice extends StatelessWidget {
  const _ElementChoice({
    required this.element,
    required this.selected,
    required this.onSelected,
  });

  final StarterElement element;
  final bool selected;
  final VoidCallback? onSelected;

  @override
  Widget build(BuildContext context) {
    final (icon, subtitle) = switch (element) {
      StarterElement.fire => (
        Icons.local_fire_department_rounded,
        'Смелый и напористый',
      ),
      StarterElement.water => (Icons.water_drop_rounded, 'Гибкий и спокойный'),
      StarterElement.earth => (Icons.landscape_rounded, 'Стойкий и надёжный'),
    };
    return Material(
      color: selected
          ? GochyaColors.primary.withValues(alpha: 0.2)
          : GochyaColors.background,
      borderRadius: BorderRadius.circular(16),
      child: InkWell(
        onTap: onSelected,
        borderRadius: BorderRadius.circular(16),
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Row(
            children: [
              Icon(
                icon,
                size: 34,
                color: selected ? GochyaColors.secondary : null,
              ),
              const SizedBox(width: 14),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      element.label,
                      style: const TextStyle(
                        fontSize: 17,
                        fontWeight: FontWeight.w800,
                      ),
                    ),
                    Text(
                      subtitle,
                      style: const TextStyle(color: GochyaColors.muted),
                    ),
                  ],
                ),
              ),
              Icon(
                selected ? Icons.check_circle_rounded : Icons.circle_outlined,
                color: selected ? GochyaColors.success : GochyaColors.muted,
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _ParentalConsentState extends StatelessWidget {
  const _ParentalConsentState();

  @override
  Widget build(BuildContext context) {
    return const Card(
      child: Padding(
        padding: EdgeInsets.all(24),
        child: Column(
          children: [
            Icon(Icons.family_restroom_rounded, size: 56),
            SizedBox(height: 16),
            Text(
              'Нужно согласие родителя',
              textAlign: TextAlign.center,
              style: TextStyle(fontSize: 22, fontWeight: FontWeight.w800),
            ),
            SizedBox(height: 10),
            Text(
              'Продолжение будет доступно после подключения проверяемого '
              'parental consent. До этого GOCHYA не создаёт питомца и не '
              'включает Health, аналитику, IAP или рейтинговый PvP.',
              textAlign: TextAlign.center,
              style: TextStyle(color: GochyaColors.muted),
            ),
          ],
        ),
      ),
    );
  }
}

class _SubmissionError extends StatelessWidget {
  const _SubmissionError({required this.error});

  final Object error;

  @override
  Widget build(BuildContext context) {
    final apiError = error is ApiException ? error as ApiException : null;
    final message = switch (apiError?.code) {
      'age_gate_locked' => 'Возрастная категория уже зафиксирована.',
      'parental_consent_required' => 'Сначала нужно согласие родителя.',
      'starter_already_selected' =>
        'Для аккаунта уже выбрано другое starter-яйцо.',
      'starter_unavailable' => 'Starter-яйцо для этого аккаунта недоступно.',
      _ => 'Не удалось сохранить выбор. Проверь соединение и повтори.',
    };
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Icon(
              Icons.error_outline_rounded,
              color: GochyaColors.warning,
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(message),
                  if (apiError?.requestId != null) ...[
                    const SizedBox(height: 4),
                    Text(
                      'Request ID: ${apiError!.requestId}',
                      style: Theme.of(context).textTheme.bodySmall,
                    ),
                  ],
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _ButtonProgress extends StatelessWidget {
  const _ButtonProgress({required this.label});

  final String label;

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        const SizedBox.square(
          dimension: 18,
          child: CircularProgressIndicator(strokeWidth: 2),
        ),
        const SizedBox(width: 10),
        Text(label),
      ],
    );
  }
}

String _formatDisplayDate(DateTime value) {
  String twoDigits(int number) => number.toString().padLeft(2, '0');
  return '${twoDigits(value.day)}.${twoDigits(value.month)}.${value.year}';
}
