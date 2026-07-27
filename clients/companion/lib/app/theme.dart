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

/// Element palette from `ART_BIBLE.md` §3.2.
///
/// A scale of its own. Before this existed the element tints were borrowed from
/// the needs palette — Water took `energy`, Earth took `hygiene`, Magma took
/// `warning` — which put back exactly the collision the §5 audit removed, only
/// from the other direction: an Earth pet was tinted mint and sat directly
/// above a mint hygiene bar.
///
/// Three elements are lightened for UI use because their canonical colour does
/// not clear the 3:1 non-text threshold on `#1A1B2E`: Mud (1.96), Eclipse
/// (1.11) and Inferno (1.69). The hue is preserved exactly, so the creature
/// still reads as the same species; only the UI tint is raised. All three are
/// phase-2/alpha elements.
abstract final class GochyaElementColors {
  static const fire = Color(0xFFFF6B35);
  static const water = Color(0xFF4ECDC4);
  static const earth = Color(0xFFC9A66B);
  static const steam = Color(0xFFE8E8E8);
  static const air = Color(0xFFB8E1FF);
  static const light = Color(0xFFFFE66D);
  static const dark = Color(0xFF9B87C4);
  static const arcane = Color(0xFFC77DFF);
  static const magma = Color(0xFFFF4500);
  static const storm = Color(0xFF5C7AEA);
  static const smoke = Color(0xFF9E9E9E);
  static const sand = Color(0xFFE6D5A8);
  static const prism = Color(0xFFF0F8FF);
  static const crystal = Color(0xFFB8E6D2);

  /// Canonical `#6B4226` — too dark for the UI at 1.96:1.
  static const mud = Color(0xFF9E6138);

  /// Canonical `#2D1B4E` — 1.11:1, effectively invisible on the dark ground.
  static const eclipse = Color(0xFF815BC6);

  /// Canonical `#8B0000` — 1.69:1.
  static const inferno = Color(0xFFE20000);
}

/// Activity ring ramp from `ART_BIBLE.md` §3.4.
///
/// Deliberately one hue at three lightness steps rather than three categorical
/// colours: needs, elements, rarities and UI states already claim every hue in
/// the palette, and minting a fourth categorical scale is exactly the collision
/// the §3.3 and §5 audits removed. The rings are told apart by radius, icon and
/// label — never by colour alone, which also satisfies the contrast rules in
/// `UX_UI.md` §10. All three clear the 3:1 non-text threshold on `#1A1B2E`.
abstract final class GochyaRingColors {
  /// Steps — outer ring.
  static const steps = Color(0xFFC4B5FD);

  /// Sleep — middle ring.
  static const sleep = Color(0xFF9B7DFF);

  /// Active calories — inner ring.
  static const calories = Color(0xFF7C5CFF);
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
