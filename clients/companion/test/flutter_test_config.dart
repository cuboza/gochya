import 'dart:async';

import 'package:flutter_test/flutter_test.dart';

/// Creature idle loops never end, so `pumpAndSettle` would time out on any
/// screen that shows a pet. Widget tests therefore run in the app's
/// reduced-motion mode — a supported production configuration, not a
/// test-only shortcut — which renders the same layout without a live ticker.
Future<void> testExecutable(FutureOr<void> Function() testMain) async {
  final binding = TestWidgetsFlutterBinding.ensureInitialized();
  binding.platformDispatcher.accessibilityFeaturesTestValue =
      const FakeAccessibilityFeatures(disableAnimations: true);
  await testMain();
}
