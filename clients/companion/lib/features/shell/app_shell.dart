import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/session/session_store.dart';
import '../home/home_screen.dart';
import '../session/session_controller.dart';
import '../shop/shop_screen.dart';

class AppShell extends StatefulWidget {
  const AppShell({required this.tokens, super.key});

  final SessionTokens tokens;

  @override
  State<AppShell> createState() => _AppShellState();
}

class _AppShellState extends State<AppShell> {
  var _selectedIndex = 0;

  @override
  Widget build(BuildContext context) {
    final page = switch (_selectedIndex) {
      0 => HomeScreen(accessToken: widget.tokens.accessToken),
      1 => ShopScreen(accessToken: widget.tokens.accessToken),
      2 => const _ComingSoonScreen(
        icon: Icons.sports_martial_arts_outlined,
        title: 'PvP',
      ),
      3 => const _ComingSoonScreen(
        icon: Icons.egg_alt_outlined,
        title: 'Бридинг',
      ),
      _ => const _ProfileScreen(),
    };

    return Scaffold(
      body: page,
      bottomNavigationBar: NavigationBar(
        selectedIndex: _selectedIndex,
        onDestinationSelected: (index) {
          setState(() => _selectedIndex = index);
        },
        destinations: const [
          NavigationDestination(
            icon: Icon(Icons.home_outlined),
            selectedIcon: Icon(Icons.home_rounded),
            label: 'Главная',
          ),
          NavigationDestination(
            icon: Icon(Icons.storefront_outlined),
            selectedIcon: Icon(Icons.storefront_rounded),
            label: 'Магазин',
          ),
          NavigationDestination(
            icon: Icon(Icons.sports_martial_arts_outlined),
            selectedIcon: Icon(Icons.sports_martial_arts_rounded),
            label: 'PvP',
          ),
          NavigationDestination(
            icon: Icon(Icons.egg_alt_outlined),
            selectedIcon: Icon(Icons.egg_alt_rounded),
            label: 'Бридинг',
          ),
          NavigationDestination(
            icon: Icon(Icons.person_outline_rounded),
            selectedIcon: Icon(Icons.person_rounded),
            label: 'Профиль',
          ),
        ],
      ),
    );
  }
}

class _ComingSoonScreen extends StatelessWidget {
  const _ComingSoonScreen({required this.icon, required this.title});

  final IconData icon;
  final String title;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: Text(title)),
      body: Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(icon, size: 64),
            const SizedBox(height: 16),
            const Text('Раздел подключается к серверному контракту'),
          ],
        ),
      ),
    );
  }
}

class _ProfileScreen extends ConsumerWidget {
  const _ProfileScreen();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return Scaffold(
      appBar: AppBar(title: const Text('Профиль')),
      body: Center(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: FilledButton.tonalIcon(
            onPressed: () {
              ref.read(sessionControllerProvider.notifier).signOut();
            },
            icon: const Icon(Icons.logout_rounded),
            label: const Text('Выйти и отозвать сессию'),
          ),
        ),
      ),
    );
  }
}
