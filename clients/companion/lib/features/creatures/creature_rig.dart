import 'dart:math';
import 'dart:ui' show Offset;

import '../../core/models/technique_models.dart';

/// Skinning rig for the flat species paintings.
///
/// The canonical pipeline (`ART_BIBLE.md` §10) authors skeletal animation in
/// Rive, which needs layered source art. The shipped species are single flat
/// paintings, so limbs are driven here by weighted mesh deformation instead:
/// each bone owns an anchor and a falloff radius, and vertices near it follow
/// its rotation and translation. That animates head, tail and legs separately
/// without cutting holes into a painting that has no layers behind them.
///
/// Motion follows `ART_BIBLE.md` §9.1: anticipation before a strike,
/// follow-through after it, and overlapping action — the tail lags the body by
/// a phase offset instead of moving with it.
class Bone {
  const Bone({
    required this.pivot,
    required this.radius,
    this.rotation = 0,
    this.translation = Offset.zero,
  });

  /// Anchor in normalized sprite space, origin at the top-left.
  final Offset pivot;

  /// Influence radius in normalized units; weight falls off smoothly to zero.
  final double radius;

  final double rotation;

  /// Normalized displacement applied to the influenced region.
  final Offset translation;

  Bone lerpFrom(double amount) => Bone(
    pivot: pivot,
    radius: radius,
    rotation: rotation * amount,
    translation: translation * amount,
  );

  /// Smooth falloff, so neighbouring vertices never tear apart.
  double weightAt(Offset point) {
    final distance = (point - pivot).distance;
    if (distance >= radius) {
      return 0;
    }
    final t = 1 - distance / radius;
    return t * t * (3 - 2 * t);
  }
}

/// Where the limbs of a species sit inside its own painting.
class CreatureAnchors {
  const CreatureAnchors({
    required this.head,
    required this.tail,
    required this.frontLimb,
    required this.hindLimb,
    this.headRadius = 0.34,
    this.tailRadius = 0.42,
    this.limbRadius = 0.26,
  });

  final Offset head;
  final Offset tail;
  final Offset frontLimb;
  final Offset hindLimb;
  final double headRadius;
  final double tailRadius;
  final double limbRadius;
}

/// Anchors were read off each shipped painting; every species faces its own
/// direction, so they cannot share one generic layout.
const _anchors = <CreatureElement, CreatureAnchors>{
  // Fox: ears top-right, bushy tail sweeping to the lower left.
  CreatureElement.fire: CreatureAnchors(
    head: Offset(0.68, 0.17),
    tail: Offset(0.18, 0.62),
    frontLimb: Offset(0.70, 0.88),
    hindLimb: Offset(0.42, 0.88),
  ),
  // Axolotl: head low on the left, tail arcing up to the right.
  CreatureElement.water: CreatureAnchors(
    head: Offset(0.24, 0.68),
    tail: Offset(0.64, 0.16),
    frontLimb: Offset(0.22, 0.90),
    hindLimb: Offset(0.72, 0.82),
  ),
  // Badger: head to the right, armoured rear to the left.
  CreatureElement.earth: CreatureAnchors(
    head: Offset(0.78, 0.34),
    tail: Offset(0.12, 0.44),
    frontLimb: Offset(0.70, 0.90),
    hindLimb: Offset(0.26, 0.90),
  ),
  // Steam hybrid: head top-left, tail trailing to the lower right.
  CreatureElement.steam: CreatureAnchors(
    head: Offset(0.32, 0.14),
    tail: Offset(0.76, 0.84),
    frontLimb: Offset(0.30, 0.92),
    hindLimb: Offset(0.62, 0.90),
  ),
};

CreatureAnchors anchorsFor(CreatureElement? element) {
  return _anchors[element] ??
      const CreatureAnchors(
        head: Offset(0.7, 0.25),
        tail: Offset(0.2, 0.55),
        frontLimb: Offset(0.68, 0.88),
        hindLimb: Offset(0.34, 0.88),
      );
}

/// What the creature is doing this frame.
enum CreatureAction {
  idle,
  strike,
  hit,
  victory,
  defeat,
  eat,
  clean,
  play,
  sleeping,
}

/// A frame of deformation: bones plus a whole-body transform.
class CreaturePose {
  const CreaturePose({
    required this.bones,
    this.bodyOffset = Offset.zero,
    this.bodyScaleX = 1,
    this.bodyScaleY = 1,
  });

  final List<Bone> bones;
  final Offset bodyOffset;
  final double bodyScaleX;
  final double bodyScaleY;
}

/// Continuous idle: breathing plus overlapping tail and head motion.
///
/// [phase] runs 0..1 and loops. The tail trails the body by a quarter cycle so
/// it reads as follow-through rather than a rigid limb.
CreaturePose idlePose(CreatureAnchors anchors, double phase) {
  final breath = sin(phase * 2 * pi);
  final lag = sin((phase - 0.25) * 2 * pi);
  return CreaturePose(
    bones: [
      Bone(
        pivot: anchors.head,
        radius: anchors.headRadius,
        rotation: breath * 0.045,
        translation: Offset(0, -breath * 0.012),
      ),
      Bone(
        pivot: anchors.tail,
        radius: anchors.tailRadius,
        rotation: lag * 0.12,
        translation: Offset(lag * 0.02, 0),
      ),
      Bone(
        pivot: anchors.frontLimb,
        radius: anchors.limbRadius,
        rotation: breath * 0.02,
      ),
      Bone(
        pivot: anchors.hindLimb,
        radius: anchors.limbRadius,
        rotation: -breath * 0.02,
      ),
    ],
    bodyScaleY: 1 + breath * 0.012,
    bodyScaleX: 1 - breath * 0.008,
    bodyOffset: Offset(0, -breath * 0.006),
  );
}

/// One strike, as a curve over `progress` in 0..1.
///
/// The shape is anticipation (pull back) → drive → impact → follow-through
/// with a small overshoot, per `ART_BIBLE.md` §9.1. Which limb leads depends on
/// the technique: kicks drive the hind limb, punches the front one, a block
/// pulls everything in.
CreaturePose strikePose(
  CreatureAnchors anchors,
  TechniqueType type,
  double progress,
) {
  // Negative during the wind-up, peaks at contact, settles with overshoot.
  final drive = progress < 0.28
      ? -_easeInOut(progress / 0.28) * 0.35
      : progress < 0.5
      ? _easeOut((progress - 0.28) / 0.22)
      : _settle((progress - 0.5) / 0.5);
  final reach = drive.clamp(-0.4, 1.2).toDouble();

  final usesLeg = type == TechniqueType.kick;
  final leadPivot = usesLeg ? anchors.hindLimb : anchors.frontLimb;
  final leadRadius = anchors.limbRadius * (usesLeg ? 1.25 : 1.0);

  final (limbRotation, limbReach, headRotation, bodyLift) = switch (type) {
    TechniqueType.jab => (-0.30, 0.13, 0.05, 0.0),
    TechniqueType.cross => (-0.42, 0.17, 0.09, -0.01),
    TechniqueType.hook => (-0.62, 0.12, 0.16, 0.0),
    TechniqueType.uppercut => (-0.75, 0.10, -0.14, -0.05),
    TechniqueType.kick => (-0.85, 0.20, 0.06, -0.03),
    TechniqueType.elbow => (0.70, 0.09, 0.12, 0.0),
    TechniqueType.block => (0.30, -0.05, -0.06, 0.02),
  };

  return CreaturePose(
    bones: [
      Bone(
        pivot: leadPivot,
        radius: leadRadius,
        rotation: limbRotation * reach,
        translation: Offset(limbReach * reach, bodyLift * reach * 2),
      ),
      Bone(
        pivot: anchors.head,
        radius: anchors.headRadius,
        rotation: headRotation * reach,
        translation: Offset(0.05 * reach, bodyLift * reach),
      ),
      // The tail keeps swinging after the body stops: overlapping action.
      Bone(
        pivot: anchors.tail,
        radius: anchors.tailRadius,
        rotation: -0.34 * _delayed(reach, progress),
        translation: Offset(-0.05 * _delayed(reach, progress), 0),
      ),
      Bone(
        pivot: usesLeg ? anchors.frontLimb : anchors.hindLimb,
        radius: anchors.limbRadius,
        rotation: 0.18 * reach,
      ),
    ],
    bodyOffset: Offset(0.09 * reach, bodyLift * reach),
    // Squash on the wind-up, stretch through contact.
    bodyScaleX: 1 + 0.05 * reach,
    bodyScaleY: 1 - 0.04 * reach,
  );
}

/// Taking a hit: recoil away, head snaps back, then recovers.
CreaturePose hitPose(CreatureAnchors anchors, double progress) {
  final recoil = progress < 0.3
      ? _easeOut(progress / 0.3)
      : _settle((progress - 0.3) / 0.7);
  return CreaturePose(
    bones: [
      Bone(
        pivot: anchors.head,
        radius: anchors.headRadius,
        rotation: -0.22 * recoil,
        translation: Offset(-0.06 * recoil, 0.02 * recoil),
      ),
      Bone(
        pivot: anchors.tail,
        radius: anchors.tailRadius,
        rotation: 0.2 * _delayed(recoil, progress),
      ),
      Bone(
        pivot: anchors.frontLimb,
        radius: anchors.limbRadius,
        rotation: -0.12 * recoil,
      ),
    ],
    bodyOffset: Offset(-0.07 * recoil, 0),
    bodyScaleX: 1 - 0.05 * recoil,
    bodyScaleY: 1 + 0.03 * recoil,
  );
}

/// Celebration and defeat, used once a match finishes.
CreaturePose outcomePose(
  CreatureAnchors anchors,
  double phase, {
  required bool won,
}) {
  final wave = sin(phase * 2 * pi);
  if (!won) {
    return CreaturePose(
      bones: [
        Bone(
          pivot: anchors.head,
          radius: anchors.headRadius,
          rotation: 0.12,
          translation: const Offset(-0.01, 0.05),
        ),
        Bone(
          pivot: anchors.tail,
          radius: anchors.tailRadius,
          rotation: 0.16 + wave * 0.02,
          translation: const Offset(0, 0.03),
        ),
      ],
      bodyOffset: const Offset(0, 0.035),
      bodyScaleY: 0.95,
      bodyScaleX: 1.02,
    );
  }
  final hop = (sin(phase * 4 * pi)).abs();
  return CreaturePose(
    bones: [
      Bone(
        pivot: anchors.head,
        radius: anchors.headRadius,
        rotation: -0.1 * wave,
        translation: Offset(0, -0.02 * hop),
      ),
      Bone(
        pivot: anchors.tail,
        radius: anchors.tailRadius,
        rotation: 0.3 * wave,
      ),
    ],
    bodyOffset: Offset(0, -0.05 * hop),
    bodyScaleY: 1 + 0.04 * hop,
    bodyScaleX: 1 - 0.02 * hop,
  );
}

/// Care reactions, timed to `ART_BIBLE.md` §9.2: feeding is a 600 ms dip,
/// playing an 800 ms double hop, cleaning a shimmy.
CreaturePose carePose(
  CreatureAnchors anchors,
  CreatureAction action,
  double progress,
) {
  switch (action) {
    case CreatureAction.eat:
      // Head dips to the food and comes back up, body follows a beat later.
      final dip = sin(progress * pi);
      return CreaturePose(
        bones: [
          Bone(
            pivot: anchors.head,
            radius: anchors.headRadius,
            rotation: 0.26 * dip,
            translation: Offset(0.02 * dip, 0.07 * dip),
          ),
          Bone(
            pivot: anchors.tail,
            radius: anchors.tailRadius,
            rotation: 0.18 * _delayed(dip, progress),
          ),
        ],
        bodyOffset: Offset(0, 0.02 * dip),
        bodyScaleY: 1 - 0.03 * dip,
        bodyScaleX: 1 + 0.02 * dip,
      );
    case CreatureAction.clean:
      // Shimmy: the body twists one way, the head and tail lag the other.
      final shimmy = sin(progress * 6 * pi) * (1 - progress);
      return CreaturePose(
        bones: [
          Bone(
            pivot: anchors.head,
            radius: anchors.headRadius,
            rotation: -0.14 * shimmy,
          ),
          Bone(
            pivot: anchors.tail,
            radius: anchors.tailRadius,
            rotation: 0.22 * shimmy,
          ),
          Bone(
            pivot: anchors.frontLimb,
            radius: anchors.limbRadius,
            rotation: 0.1 * shimmy,
          ),
        ],
        bodyOffset: Offset(0.012 * shimmy, 0),
        bodyScaleX: 1 + 0.02 * shimmy.abs(),
      );
    case CreatureAction.play:
      // Two hops with squash on landing.
      final hop = (sin(progress * 2 * pi)).abs() * (1 - progress * 0.35);
      final squash = max(0.0, -sin(progress * 2 * pi)) * 0.6;
      return CreaturePose(
        bones: [
          Bone(
            pivot: anchors.head,
            radius: anchors.headRadius,
            rotation: -0.12 * hop,
            translation: Offset(0, -0.03 * hop),
          ),
          Bone(
            pivot: anchors.tail,
            radius: anchors.tailRadius,
            rotation: 0.4 * sin(progress * 8 * pi) * (1 - progress),
          ),
          Bone(
            pivot: anchors.frontLimb,
            radius: anchors.limbRadius,
            rotation: -0.2 * hop,
          ),
          Bone(
            pivot: anchors.hindLimb,
            radius: anchors.limbRadius,
            rotation: 0.16 * hop,
          ),
        ],
        bodyOffset: Offset(0, -0.09 * hop),
        bodyScaleY: 1 + 0.05 * hop - 0.06 * squash,
        bodyScaleX: 1 - 0.03 * hop + 0.05 * squash,
      );
    case CreatureAction.sleeping:
      // Continuous: settled low, breathing slower and deeper than awake idle.
      final breath = sin(progress * 2 * pi);
      return CreaturePose(
        bones: [
          Bone(
            pivot: anchors.head,
            radius: anchors.headRadius,
            rotation: 0.16,
            translation: Offset(-0.01, 0.055 + breath * 0.006),
          ),
          Bone(
            pivot: anchors.tail,
            radius: anchors.tailRadius,
            rotation: 0.2 + breath * 0.03,
            translation: const Offset(0, 0.03),
          ),
        ],
        bodyOffset: Offset(0, 0.05 - breath * 0.008),
        bodyScaleY: 0.94 + breath * 0.018,
        bodyScaleX: 1.03 - breath * 0.01,
      );
    case CreatureAction.idle:
    case CreatureAction.strike:
    case CreatureAction.hit:
    case CreatureAction.victory:
    case CreatureAction.defeat:
      return const CreaturePose(bones: []);
  }
}

/// Trails [value] by a fraction of the action, so a limb settles after the
/// body it hangs from.
double _delayed(double value, double progress) =>
    value * (0.45 + 0.55 * _easeOut(progress.clamp(0.0, 1.0)));

double _easeOut(double t) {
  final clamped = t.clamp(0.0, 1.0);
  return 1 - pow(1 - clamped, 3).toDouble();
}

double _easeInOut(double t) {
  final clamped = t.clamp(0.0, 1.0);
  return clamped < 0.5
      ? 2 * clamped * clamped
      : 1 - pow(-2 * clamped + 2, 2).toDouble() / 2;
}

/// Decays back to rest with one small overshoot — follow-through.
double _settle(double t) {
  final clamped = t.clamp(0.0, 1.0);
  return cos(clamped * pi * 1.5) * exp(-clamped * 3.2);
}
