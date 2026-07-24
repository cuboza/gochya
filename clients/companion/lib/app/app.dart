import 'package:flutter/material.dart';

import '../features/session/session_gate.dart';
import 'theme.dart';

class GochyaApp extends StatelessWidget {
  const GochyaApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'GOCHYA',
      debugShowCheckedModeBanner: false,
      theme: buildGochyaTheme(),
      home: const SessionGate(),
    );
  }
}
