import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../app/theme.dart';
import '../shell/app_shell.dart';
import 'session_controller.dart';

class SessionGate extends ConsumerWidget {
  const SessionGate({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final session = ref.watch(sessionControllerProvider);
    return session.when(
      loading: () => const _SessionLoadingScreen(),
      error: (error, stackTrace) => _SessionStorageErrorScreen(
        onRetry: ref.read(sessionControllerProvider.notifier).retry,
      ),
      data: (tokens) {
        if (tokens == null) {
          return const _SignedOutScreen();
        }
        return AppShell(tokens: tokens);
      },
    );
  }
}

class _SessionLoadingScreen extends StatelessWidget {
  const _SessionLoadingScreen();

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: Center(
        child: Semantics(
          label: 'Загрузка защищённой сессии',
          child: const CircularProgressIndicator(),
        ),
      ),
    );
  }
}

class _SessionStorageErrorScreen extends StatelessWidget {
  const _SessionStorageErrorScreen({required this.onRetry});

  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    return _CenteredState(
      icon: Icons.lock_outline_rounded,
      title: 'Не удалось открыть сессию',
      message:
          'GOCHYA не продолжит без безопасного доступа к хранилищу токенов.',
      action: FilledButton.icon(
        onPressed: onRetry,
        icon: const Icon(Icons.refresh_rounded),
        label: const Text('Повторить'),
      ),
    );
  }
}

class _SignedOutScreen extends StatelessWidget {
  const _SignedOutScreen();

  @override
  Widget build(BuildContext context) {
    return const _CenteredState(
      icon: Icons.pets_rounded,
      iconColor: GochyaColors.primary,
      title: 'GOCHYA',
      message:
          'Защищённая сессия не найдена. Вход через провайдера появится '
          'после подключения production OAuth-конфигурации.',
      action: Chip(
        avatar: Icon(Icons.shield_outlined, size: 18),
        label: Text('Токены не вводятся вручную'),
      ),
    );
  }
}

class _CenteredState extends StatelessWidget {
  const _CenteredState({
    required this.icon,
    required this.title,
    required this.message,
    required this.action,
    this.iconColor,
  });

  final IconData icon;
  final Color? iconColor;
  final String title;
  final String message;
  final Widget action;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: SafeArea(
        child: Center(
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(32),
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 420),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Icon(icon, size: 72, color: iconColor),
                  const SizedBox(height: 20),
                  Text(
                    title,
                    textAlign: TextAlign.center,
                    style: Theme.of(context).textTheme.headlineMedium?.copyWith(
                      fontWeight: FontWeight.w800,
                    ),
                  ),
                  const SizedBox(height: 12),
                  Text(
                    message,
                    textAlign: TextAlign.center,
                    style: Theme.of(context).textTheme.bodyLarge?.copyWith(
                      color: GochyaColors.muted,
                      height: 1.4,
                    ),
                  ),
                  const SizedBox(height: 24),
                  action,
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}
