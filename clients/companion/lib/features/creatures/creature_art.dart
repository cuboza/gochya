import 'package:flutter/material.dart';

import '../../app/theme.dart';
import '../../core/models/profile_models.dart';
import '../../core/models/technique_models.dart';

/// Canonical MVP species art (`ELEMENTAL_CREATURES_GROUNDED_V3`). Elements
/// without shipped art fall back to a tinted silhouette rather than borrowing
/// another species' portrait.
const _creatureAssets = <CreatureElement, String>{
  CreatureElement.fire: 'assets/creatures/fire.png',
  CreatureElement.water: 'assets/creatures/water.png',
  CreatureElement.earth: 'assets/creatures/earth.png',
  CreatureElement.steam: 'assets/creatures/steam.png',
};

String? creatureAssetFor(CreatureElement element) => _creatureAssets[element];

/// The profile contract keeps `genome` an opaque map, so the element is read
/// defensively here: an unexpected shape degrades to the fallback art instead
/// of failing a screen that is otherwise valid.
///
/// Both encodings the server accepts are handled — the numeric protocol id it
/// writes today, and the variant name older rows may still carry.
CreatureElement? creatureElementOf(JsonMap genome) {
  final raw = genome['element'];
  if (raw is int) {
    try {
      return CreatureElement.fromApi(raw);
    } on FormatException {
      return null;
    }
  }
  if (raw is String) {
    final name = raw.toLowerCase();
    for (final element in CreatureElement.values) {
      if (element.name == name) {
        return element;
      }
    }
  }
  return null;
}

extension CreatureElementStyle on CreatureElement {
  /// Every element carries its own colour from `ART_BIBLE.md` §3.2. None of
  /// them may double as a need, rarity or UI-state colour — see the note on
  /// [GochyaElementColors] for what happened when they did.
  Color get tint => switch (this) {
    CreatureElement.fire => GochyaElementColors.fire,
    CreatureElement.water => GochyaElementColors.water,
    CreatureElement.earth => GochyaElementColors.earth,
    CreatureElement.steam => GochyaElementColors.steam,
    CreatureElement.air => GochyaElementColors.air,
    CreatureElement.light => GochyaElementColors.light,
    CreatureElement.dark => GochyaElementColors.dark,
    CreatureElement.arcane => GochyaElementColors.arcane,
    CreatureElement.magma => GochyaElementColors.magma,
    CreatureElement.mud => GochyaElementColors.mud,
    CreatureElement.storm => GochyaElementColors.storm,
    CreatureElement.smoke => GochyaElementColors.smoke,
    CreatureElement.sand => GochyaElementColors.sand,
    CreatureElement.eclipse => GochyaElementColors.eclipse,
    CreatureElement.inferno => GochyaElementColors.inferno,
    CreatureElement.prism => GochyaElementColors.prism,
    CreatureElement.crystal => GochyaElementColors.crystal,
  };
}

/// Square creature portrait used by the pet hero and the card tiles.
class CreatureAvatar extends StatelessWidget {
  const CreatureAvatar({
    required this.element,
    required this.size,
    this.padding = 0.08,
    super.key,
  });

  final CreatureElement? element;
  final double size;

  /// Inset of the artwork inside its frame, as a fraction of [size].
  final double padding;

  @override
  Widget build(BuildContext context) {
    final tint = element?.tint ?? GochyaColors.primary;
    final asset = element == null ? null : creatureAssetFor(element!);
    return Container(
      width: size,
      height: size,
      decoration: BoxDecoration(
        gradient: LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [
            tint.withValues(alpha: 0.34),
            GochyaColors.background.withValues(alpha: 0.65),
          ],
        ),
        borderRadius: BorderRadius.circular(size * 0.3),
      ),
      padding: EdgeInsets.all(size * padding),
      child: asset == null
          ? Icon(Icons.pets_rounded, size: size * 0.52, color: tint)
          : CreatureImage(asset: asset, width: size),
    );
  }
}

/// Decodes the portrait at the size it is drawn at, so a 48px thumbnail does
/// not keep a full-resolution bitmap in memory.
class CreatureImage extends StatelessWidget {
  const CreatureImage({
    required this.asset,
    required this.width,
    this.fit = BoxFit.contain,
    super.key,
  });

  final String asset;
  final double width;
  final BoxFit fit;

  @override
  Widget build(BuildContext context) {
    final ratio = MediaQuery.devicePixelRatioOf(context);
    return Image.asset(
      asset,
      fit: fit,
      cacheWidth: (width * ratio).round(),
      filterQuality: FilterQuality.medium,
      excludeFromSemantics: true,
    );
  }
}
