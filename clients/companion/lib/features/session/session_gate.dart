import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:sign_in_with_apple/sign_in_with_apple.dart';

import '../../app/theme.dart';
import '../../core/session/session_store.dart';
import '../../core/network/gochya_api_client.dart';
import '../auth/apple_identity_client.dart';
import '../auth/auth_repository.dart';
import '../auth/google_identity_client.dart';
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

class _SignedOutScreen extends ConsumerStatefulWidget {
  const _SignedOutScreen();

  @override
  ConsumerState<_SignedOutScreen> createState() => _SignedOutScreenState();
}

class _SignedOutScreenState extends ConsumerState<_SignedOutScreen> {
  bool _isSigningIn = false;
  String? _errorMessage;

  @override
  Widget build(BuildContext context) {
    final repository = ref.watch(authRepositoryProvider);
    final isGoogleAvailable = repository.isGoogleSignInAvailable;
    final isAppleAvailable =
        ref.watch(appleSignInAvailabilityProvider).value ?? false;
    final isAvailable = isGoogleAvailable || isAppleAvailable;
    return _CenteredState(
      icon: Icons.pets_rounded,
      iconColor: GochyaColors.primary,
      title: 'GOCHYA',
      message: isAvailable
          ? 'Войдите через провайдера, чтобы безопасно подключить питомца '
                'к этому устройству.'
          : 'Защищённая сессия не найдена. Вход через провайдера появится '
                'после подключения production OAuth-конфигурации.',
      action: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          if (isAppleAvailable)
            SignInWithAppleButton(
              onPressed: _isSigningIn ? null : _signInWithApple,
              text: 'Войти через Apple',
              height: 48,
            ),
          if (isAppleAvailable && isGoogleAvailable) const SizedBox(height: 12),
          if (isGoogleAvailable)
            FilledButton.icon(
              onPressed: _isSigningIn ? null : _signInWithGoogle,
              icon: _isSigningIn
                  ? const SizedBox.square(
                      dimension: 18,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    )
                  : const Icon(Icons.login_rounded),
              label: Text(_isSigningIn ? 'Входим…' : 'Войти через Google'),
            ),
          if (!isAvailable)
            const Chip(
              avatar: Icon(Icons.shield_outlined, size: 18),
              label: Text('Токены не вводятся вручную'),
            ),
          if (_errorMessage != null) ...[
            const SizedBox(height: 16),
            Text(
              _errorMessage!,
              textAlign: TextAlign.center,
              style: TextStyle(color: Theme.of(context).colorScheme.error),
            ),
          ],
        ],
      ),
    );
  }

  Future<void> _signInWithApple() async {
    setState(() {
      _isSigningIn = true;
      _errorMessage = null;
    });
    try {
      final tokens = await ref.read(authRepositoryProvider).signInWithApple();
      await _saveSession(tokens);
    } on AppleIdentityException catch (error) {
      if (!mounted || error.failure == AppleIdentityFailure.cancelled) {
        return;
      }
      setState(() {
        _errorMessage = switch (error.failure) {
          AppleIdentityFailure.unavailable =>
            'Вход через Apple сейчас недоступен. Попробуйте позже.',
          AppleIdentityFailure.missingIdentityToken =>
            'Apple не выдала подтверждение личности. Повторите вход.',
          AppleIdentityFailure.cancelled => null,
        };
      });
    } on ApiException catch (error) {
      _showApiError(error, providerName: 'Apple');
    } on Object {
      _showUnexpectedError();
    } finally {
      _finishSignIn();
    }
  }

  Future<void> _signInWithGoogle() async {
    setState(() {
      _isSigningIn = true;
      _errorMessage = null;
    });
    try {
      final tokens = await ref.read(authRepositoryProvider).signInWithGoogle();
      await _saveSession(tokens);
    } on GoogleIdentityException catch (error) {
      if (!mounted) {
        return;
      }
      if (error.failure != GoogleIdentityFailure.cancelled) {
        setState(() {
          _errorMessage = switch (error.failure) {
            GoogleIdentityFailure.configuration =>
              'Google OAuth настроен неверно. Попробуйте позже.',
            GoogleIdentityFailure.unavailable =>
              'Сервис Google сейчас недоступен. Попробуйте позже.',
            GoogleIdentityFailure.missingIdToken =>
              'Google не выдал подтверждение личности. Повторите вход.',
            GoogleIdentityFailure.cancelled => null,
          };
        });
      }
    } on ApiException catch (error) {
      _showApiError(error, providerName: 'Google');
    } on Object {
      _showUnexpectedError();
    } finally {
      _finishSignIn();
    }
  }

  Future<void> _saveSession(SessionTokens tokens) async {
    if (!mounted) {
      return;
    }
    await ref.read(sessionControllerProvider.notifier).save(tokens);
  }

  void _showApiError(ApiException error, {required String providerName}) {
    if (!mounted) {
      return;
    }
    setState(() {
      _errorMessage = switch (error.code) {
        'login_nonce_invalid' =>
          'Срок запроса $providerName истёк. Повторите вход.',
        'identity_token_invalid' =>
          '$providerName не подтвердил эту попытку входа. Повторите ещё раз.',
        'identity_provider_unavailable' =>
          'Сервис входа временно недоступен. Попробуйте позже.',
        'request_timeout' || 'network_error' =>
          'Нет связи с GOCHYA. Проверьте интернет и повторите попытку.',
        _ => 'Не удалось войти. Повторите попытку.',
      };
    });
  }

  void _showUnexpectedError() {
    if (!mounted) {
      return;
    }
    setState(() {
      _errorMessage = 'Не удалось войти. Повторите попытку.';
    });
  }

  void _finishSignIn() {
    if (mounted) {
      setState(() {
        _isSigningIn = false;
      });
    }
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
