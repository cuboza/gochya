import 'dart:math' as math;
import 'dart:typed_data';
import 'dart:ui' as ui;

import 'package:flutter/material.dart';

import '../../core/models/technique_models.dart';
import 'creature_art.dart';
import 'creature_rig.dart';

/// Grid resolution of the deformation mesh. 14×14 keeps limb bends smooth and
/// still costs only a few hundred vertices per frame.
const _gridSize = 14;

/// A species painting rendered as a skinned mesh, so head, tail and legs move
/// independently instead of the whole sprite sliding as one rectangle.
class RiggedCreature extends StatefulWidget {
  const RiggedCreature({
    required this.element,
    required this.width,
    this.facingLeft = false,
    this.action = CreatureAction.idle,
    this.strike,
    this.actionProgress = 0,
    this.idle = true,
    super.key,
  });

  final CreatureElement? element;
  final double width;

  /// Species art faces its own way; a duel mirrors one of the two.
  final bool facingLeft;

  final CreatureAction action;

  /// Which strike is being thrown, when [action] is `strike`.
  final TechniqueType? strike;

  /// Progress of [action] in 0..1, driven by the caller.
  final double actionProgress;

  /// Whether the looping idle motion runs on top of the action.
  final bool idle;

  @override
  State<RiggedCreature> createState() => _RiggedCreatureState();
}

class _RiggedCreatureState extends State<RiggedCreature>
    with SingleTickerProviderStateMixin {
  late final AnimationController _idle = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 2600),
  );
  ImageStream? _stream;
  ImageStreamListener? _listener;
  ui.Image? _image;
  String? _asset;

  @override
  void initState() {
    super.initState();
    _idle.repeat();
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    _resolveImage();
  }

  @override
  void didUpdateWidget(covariant RiggedCreature oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.element != widget.element) {
      _resolveImage();
    }
  }

  @override
  void dispose() {
    _detach();
    _idle.dispose();
    super.dispose();
  }

  void _detach() {
    if (_listener case final listener?) {
      _stream?.removeListener(listener);
    }
    _listener = null;
    _stream = null;
  }

  void _resolveImage() {
    final element = widget.element;
    final asset = element == null ? null : creatureAssetFor(element);
    if (asset == _asset && _image != null) {
      return;
    }
    _asset = asset;
    _detach();
    if (asset == null) {
      setState(() => _image = null);
      return;
    }
    final stream = AssetImage(asset).resolve(
      createLocalImageConfiguration(context, size: Size.square(widget.width)),
    );
    final listener = ImageStreamListener((info, _) {
      if (!mounted) {
        return;
      }
      setState(() => _image = info.image);
    });
    _stream = stream..addListener(listener);
    _listener = listener;
  }

  @override
  Widget build(BuildContext context) {
    final reduceMotion = MediaQuery.disableAnimationsOf(context);
    if (reduceMotion && _idle.isAnimating) {
      _idle.stop();
    } else if (!reduceMotion && !_idle.isAnimating) {
      _idle.repeat();
    }

    final image = _image;
    if (image == null) {
      return SizedBox(
        width: widget.width,
        height: widget.width,
        child: Icon(
          Icons.pets_rounded,
          size: widget.width * 0.6,
          color: widget.element?.tint ?? Colors.white24,
        ),
      );
    }

    final height = widget.width * image.height / image.width;
    return SizedBox(
      width: widget.width,
      height: height,
      child: AnimatedBuilder(
        animation: _idle,
        builder: (context, _) {
          return CustomPaint(
            painter: _SkinnedPainter(
              image: image,
              anchors: anchorsFor(widget.element),
              phase: reduceMotion || !widget.idle ? 0.25 : _idle.value,
              action: widget.action,
              strike: widget.strike,
              progress: widget.actionProgress,
              facingLeft: widget.facingLeft,
              animateIdle: !reduceMotion && widget.idle,
            ),
          );
        },
      ),
    );
  }
}

class _SkinnedPainter extends CustomPainter {
  _SkinnedPainter({
    required this.image,
    required this.anchors,
    required this.phase,
    required this.action,
    required this.strike,
    required this.progress,
    required this.facingLeft,
    required this.animateIdle,
  });

  final ui.Image image;
  final CreatureAnchors anchors;
  final double phase;
  final CreatureAction action;
  final TechniqueType? strike;
  final double progress;
  final bool facingLeft;
  final bool animateIdle;

  static final _mesh = _SpriteMesh.build(_gridSize);

  @override
  void paint(Canvas canvas, Size size) {
    final pose = _composePose();

    if (facingLeft) {
      canvas.save();
      canvas.translate(size.width, 0);
      canvas.scale(-1, 1);
    }

    final positions = Float32List(_mesh.unit.length * 2);
    for (var index = 0; index < _mesh.unit.length; index++) {
      final point = _deform(_mesh.unit[index], pose);
      positions[index * 2] = point.dx * size.width;
      positions[index * 2 + 1] = point.dy * size.height;
    }

    final vertices = ui.Vertices.raw(
      ui.VertexMode.triangles,
      positions,
      textureCoordinates: _mesh.texture(image.width, image.height),
      indices: _mesh.indices,
    );
    final shader = ui.ImageShader(
      image,
      TileMode.clamp,
      TileMode.clamp,
      Matrix4.identity().storage,
      filterQuality: FilterQuality.medium,
    );
    canvas.drawVertices(vertices, BlendMode.srcOver, Paint()..shader = shader);

    if (facingLeft) {
      canvas.restore();
    }
  }

  CreaturePose _composePose() {
    final idle = animateIdle && action != CreatureAction.sleeping
        ? idlePose(anchors, phase)
        : const CreaturePose(bones: []);
    final overlay = switch (action) {
      CreatureAction.idle => null,
      CreatureAction.strike => strikePose(
        anchors,
        strike ?? TechniqueType.jab,
        progress,
      ),
      CreatureAction.hit => hitPose(anchors, progress),
      CreatureAction.victory => outcomePose(anchors, phase, won: true),
      CreatureAction.defeat => outcomePose(anchors, phase, won: false),
      // Sleep is continuous, so it reads the looping phase, not the action
      // progress, and replaces the awake idle entirely.
      CreatureAction.sleeping => carePose(anchors, action, phase),
      CreatureAction.eat ||
      CreatureAction.clean ||
      CreatureAction.play => carePose(anchors, action, progress),
    };
    if (overlay == null) {
      return idle;
    }
    return CreaturePose(
      bones: [...idle.bones, ...overlay.bones],
      bodyOffset: idle.bodyOffset + overlay.bodyOffset,
      bodyScaleX: idle.bodyScaleX * overlay.bodyScaleX,
      bodyScaleY: idle.bodyScaleY * overlay.bodyScaleY,
    );
  }

  /// Linear blend of bone displacements, then the whole-body transform.
  ///
  /// Displacements are additive rather than normalized: amplitudes stay small,
  /// and additive blending keeps the mesh continuous where influences overlap.
  Offset _deform(Offset point, CreaturePose pose) {
    var moved = point;
    for (final bone in pose.bones) {
      final weight = bone.weightAt(point);
      if (weight == 0) {
        continue;
      }
      final local = point - bone.pivot;
      final rotated = bone.rotation == 0
          ? local
          : Offset(
              local.dx * math.cos(bone.rotation) -
                  local.dy * math.sin(bone.rotation),
              local.dx * math.sin(bone.rotation) +
                  local.dy * math.cos(bone.rotation),
            );
      final target = bone.pivot + rotated + bone.translation;
      moved += (target - point) * weight;
    }
    // Body transform pivots on the feet, so scaling never lifts the creature
    // off the ground.
    const ground = Offset(0.5, 1);
    final scaled = Offset(
      ground.dx + (moved.dx - ground.dx) * pose.bodyScaleX,
      ground.dy + (moved.dy - ground.dy) * pose.bodyScaleY,
    );
    return scaled + pose.bodyOffset;
  }

  @override
  bool shouldRepaint(_SkinnedPainter oldDelegate) =>
      oldDelegate.phase != phase ||
      oldDelegate.progress != progress ||
      oldDelegate.action != action ||
      oldDelegate.strike != strike ||
      oldDelegate.image != image ||
      oldDelegate.facingLeft != facingLeft ||
      oldDelegate.animateIdle != animateIdle;
}

/// Precomputed grid: unit-space vertices, triangle indices and the texture
/// coordinates that map them back onto the sprite.
class _SpriteMesh {
  _SpriteMesh._(this.unit, this.indices, this._uv);

  factory _SpriteMesh.build(int divisions) {
    final unit = <Offset>[];
    for (var row = 0; row <= divisions; row++) {
      for (var column = 0; column <= divisions; column++) {
        unit.add(Offset(column / divisions, row / divisions));
      }
    }
    final indices = <int>[];
    for (var row = 0; row < divisions; row++) {
      for (var column = 0; column < divisions; column++) {
        final topLeft = row * (divisions + 1) + column;
        final topRight = topLeft + 1;
        final bottomLeft = topLeft + divisions + 1;
        final bottomRight = bottomLeft + 1;
        indices.addAll([
          topLeft,
          topRight,
          bottomLeft,
          topRight,
          bottomRight,
          bottomLeft,
        ]);
      }
    }
    return _SpriteMesh._(
      List.unmodifiable(unit),
      Uint16List.fromList(indices),
      Float32List(unit.length * 2),
    );
  }

  final List<Offset> unit;
  final Uint16List indices;
  final Float32List _uv;
  int _uvWidth = -1;
  int _uvHeight = -1;

  Float32List texture(int width, int height) {
    if (_uvWidth == width && _uvHeight == height) {
      return _uv;
    }
    for (var index = 0; index < unit.length; index++) {
      _uv[index * 2] = unit[index].dx * width;
      _uv[index * 2 + 1] = unit[index].dy * height;
    }
    _uvWidth = width;
    _uvHeight = height;
    return _uv;
  }
}
