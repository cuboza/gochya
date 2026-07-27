import 'package:flutter/material.dart';

import '../../core/session/session_store.dart';
import '../battle/battle_screen.dart';
import '../breeding/breeding_screen.dart';
import '../home/home_screen.dart';
import '../profile/profile_screen.dart';
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
      2 => BattleScreen(accessToken: widget.tokens.accessToken),
      3 => BreedingScreen(accessToken: widget.tokens.accessToken),
      _ => ProfileScreen(accessToken: widget.tokens.accessToken),
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
