import 'package:flutter/material.dart';

abstract final class GochyaColors {
  static const background = Color(0xFF1A1B2E);
  static const backgroundMid = Color(0xFF2A2D4F);
  static const surface = Color(0xFFF4F1E8);
  static const primary = Color(0xFF9B7DFF);
  static const secondary = Color(0xFFFFD166);
  static const success = Color(0xFF06D6A0);
  static const warning = Color(0xFFEF476F);
  static const muted = Color(0xFF9B9DB8);

  static const hunger = Color(0xFFF4A259);
  static const energy = Color(0xFF5B8DEF);
  static const hygiene = Color(0xFF6FE0C6);
  static const mood = Color(0xFFFF8FB1);
}

/// Rarity frame palette from `ART_BIBLE.md` §3.3. It is a scale of its own:
/// none of these colours may double as an element or a UI-state colour, and
/// Epic is frame-only because it does not reach the 4.5:1 text threshold.
abstract final class GochyaRarityColors {
  static const common = Color(0xFF8A8FA3);
  static const uncommon = Color(0xFF4CAF6E);
  static const rare = Color(0xFF3D8BFD);
  static const epic = Color(0xFFA855F7);
  static const legendary = Color(0xFFFF9F1C);
  static const mythic = Color(0xFFF5F0FF);
}

ThemeData buildGochyaTheme() {
  final colorScheme = ColorScheme.fromSeed(
    seedColor: GochyaColors.primary,
    brightness: Brightness.dark,
    primary: GochyaColors.primary,
    secondary: GochyaColors.secondary,
    surface: GochyaColors.backgroundMid,
    error: GochyaColors.warning,
  );

  return ThemeData(
    useMaterial3: true,
    colorScheme: colorScheme,
    scaffoldBackgroundColor: GochyaColors.background,
    cardTheme: const CardThemeData(
      color: GochyaColors.backgroundMid,
      elevation: 0,
      margin: EdgeInsets.zero,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.all(Radius.circular(20)),
      ),
    ),
    navigationBarTheme: NavigationBarThemeData(
      backgroundColor: GochyaColors.backgroundMid,
      indicatorColor: GochyaColors.primary.withValues(alpha: 0.24),
      labelTextStyle: WidgetStateProperty.resolveWith((states) {
        return TextStyle(
          color: states.contains(WidgetState.selected)
              ? Colors.white
              : GochyaColors.muted,
          fontSize: 11,
          fontWeight: states.contains(WidgetState.selected)
              ? FontWeight.w700
              : FontWeight.w500,
        );
      }),
    ),
    inputDecorationTheme: const InputDecorationTheme(
      filled: true,
      fillColor: GochyaColors.backgroundMid,
      border: OutlineInputBorder(
        borderRadius: BorderRadius.all(Radius.circular(16)),
        borderSide: BorderSide.none,
      ),
    ),
  );
}
