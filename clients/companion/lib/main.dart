import 'package:flutter/foundation.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'app/app.dart';
import 'dev/demo_mode.dart';

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  const demoPlayerRequested = bool.fromEnvironment('GOCHYA_DEMO_PLAYER');
  const app = GochyaApp();
  runApp(
    kDebugMode && demoPlayerRequested
        ? const DemoPlayerScope(child: app)
        : const ProviderScope(child: app),
  );
}
