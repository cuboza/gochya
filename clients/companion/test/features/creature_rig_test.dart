import 'package:flutter_test/flutter_test.dart';
import 'package:gochya_companion/core/models/technique_models.dart';
import 'package:gochya_companion/features/creatures/creature_rig.dart';

void main() {
  const anchors = CreatureAnchors(
    head: Offset(0.7, 0.2),
    tail: Offset(0.2, 0.6),
    frontLimb: Offset(0.7, 0.9),
    hindLimb: Offset(0.3, 0.9),
  );

  group('bone weights', () {
    const bone = Bone(pivot: Offset(0.5, 0.5), radius: 0.3);

    test('are full at the pivot and vanish at the radius', () {
      expect(bone.weightAt(const Offset(0.5, 0.5)), 1);
      expect(bone.weightAt(const Offset(0.8, 0.5)), 0);
      expect(bone.weightAt(const Offset(0.95, 0.95)), 0);
    });

    test('fall off monotonically, so the mesh cannot tear', () {
      var previous = bone.weightAt(const Offset(0.5, 0.5));
      for (var step = 1; step <= 10; step++) {
        final weight = bone.weightAt(Offset(0.5 + 0.03 * step, 0.5));
        expect(weight, lessThanOrEqualTo(previous));
        expect(weight, inInclusiveRange(0, 1));
        previous = weight;
      }
    });
  });

  group('strike', () {
    test('anticipates before it drives', () {
      // ART_BIBLE §9.1: the limb pulls back first, then swings through.
      final windUp = _leadRotation(anchors, TechniqueType.jab, 0.15);
      final contact = _leadRotation(anchors, TechniqueType.jab, 0.45);
      expect(windUp.sign, isNot(contact.sign));
      expect(contact.abs(), greaterThan(windUp.abs()));
    });

    test('settles back toward rest after contact', () {
      final contact = _leadRotation(anchors, TechniqueType.kick, 0.5);
      final recovered = _leadRotation(anchors, TechniqueType.kick, 0.95);
      expect(recovered.abs(), lessThan(contact.abs()));
    });

    test('kicks lead with the hind limb, punches with the front one', () {
      final kick = strikePose(anchors, TechniqueType.kick, 0.5);
      final jab = strikePose(anchors, TechniqueType.jab, 0.5);
      expect(kick.bones.first.pivot, anchors.hindLimb);
      expect(jab.bones.first.pivot, anchors.frontLimb);
    });

    test('every strike stays inside a sane deformation budget', () {
      for (final type in TechniqueType.values) {
        for (var step = 0; step <= 20; step++) {
          final pose = strikePose(anchors, type, step / 20);
          for (final bone in pose.bones) {
            expect(bone.rotation.isFinite, isTrue, reason: type.name);
            expect(bone.rotation.abs(), lessThan(1.6), reason: type.name);
            expect(bone.translation.distance, lessThan(0.5), reason: type.name);
          }
          expect(pose.bodyScaleX, inInclusiveRange(0.8, 1.2));
          expect(pose.bodyScaleY, inInclusiveRange(0.8, 1.2));
        }
      }
    });
  });

  test('idle keeps breathing and trails the tail behind the body', () {
    final quarter = idlePose(anchors, 0.25);
    final tail = quarter.bones.firstWhere((b) => b.pivot == anchors.tail);
    final head = quarter.bones.firstWhere((b) => b.pivot == anchors.head);
    // At peak inhale the head leads; the tail is a quarter cycle behind it.
    expect(head.rotation.abs(), greaterThan(0));
    expect(tail.rotation.abs(), lessThan(head.rotation.abs()));
    expect(quarter.bodyScaleY, greaterThan(1));
  });

  test('hit recoils away from the attacker and recovers', () {
    final struck = hitPose(anchors, 0.25);
    final settled = hitPose(anchors, 0.95);
    expect(struck.bodyOffset.dx, lessThan(0));
    expect(settled.bodyOffset.dx.abs(), lessThan(struck.bodyOffset.dx.abs()));
  });

  test('outcome poses differ for a winner and a loser', () {
    final won = outcomePose(anchors, 0.3, won: true);
    final lost = outcomePose(anchors, 0.3, won: false);
    expect(won.bodyOffset.dy, lessThan(0));
    expect(lost.bodyOffset.dy, greaterThan(0));
  });

  test('every shipped species has its own anchor layout', () {
    final layouts = <Offset>{};
    for (final element in [
      CreatureElement.fire,
      CreatureElement.water,
      CreatureElement.earth,
      CreatureElement.steam,
    ]) {
      final anchors = anchorsFor(element);
      layouts.add(anchors.head);
      for (final point in [
        anchors.head,
        anchors.tail,
        anchors.frontLimb,
        anchors.hindLimb,
      ]) {
        expect(point.dx, inInclusiveRange(0, 1));
        expect(point.dy, inInclusiveRange(0, 1));
      }
    }
    expect(layouts, hasLength(4));
  });
}

double _leadRotation(
  CreatureAnchors anchors,
  TechniqueType type,
  double progress,
) {
  return strikePose(anchors, type, progress).bones.first.rotation;
}
