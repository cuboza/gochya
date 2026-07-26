import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gochya_companion/app/theme.dart';
import 'package:gochya_companion/features/home/need_indicator.dart';

void main() {
  testWidgets('calls out a need below the threshold', (tester) async {
    await _pump(tester, value: 12);

    expect(find.text('мало'), findsOneWidget);
    expect(find.byIcon(Icons.priority_high_rounded), findsOneWidget);
  });

  testWidgets('stays quiet at and above the threshold', (tester) async {
    await _pump(tester, value: lowNeedThreshold);

    expect(find.text('мало'), findsNothing);
    expect(find.byIcon(Icons.priority_high_rounded), findsNothing);
  });

  testWidgets('the marker is not motion-only', (tester) async {
    // Reduced motion is the suite default. The static marker must survive it,
    // otherwise a player who cannot see movement loses the signal entirely.
    await _pump(tester, value: 5);

    expect(find.text('мало'), findsOneWidget);
  });

  testWidgets('the pulse never fades the bar out of sight', (tester) async {
    await _pump(tester, value: 5, reducedMotion: false);
    await tester.pump(const Duration(milliseconds: 600));

    final opacity = tester.widget<Opacity>(
      find
          .ancestor(
            of: find.byType(LinearProgressIndicator),
            matching: find.byType(Opacity),
          )
          .first,
    );
    expect(opacity.opacity, greaterThanOrEqualTo(0.55));
  });

  testWidgets('a low need says so to a screen reader', (tester) async {
    final handle = tester.ensureSemantics();
    await _pump(tester, value: 8);

    expect(find.bySemanticsLabel('Сытость: 8 из 100, мало'), findsOneWidget);
    handle.dispose();
  });

  testWidgets('a healthy need is announced without the marker', (tester) async {
    final handle = tester.ensureSemantics();
    await _pump(tester, value: 81);

    expect(find.bySemanticsLabel('Сытость: 81 из 100'), findsOneWidget);
    handle.dispose();
  });
}

Future<void> _pump(
  WidgetTester tester, {
  required int value,
  bool reducedMotion = true,
}) async {
  await tester.pumpWidget(
    MaterialApp(
      theme: buildGochyaTheme(),
      home: MediaQuery(
        data: MediaQueryData(disableAnimations: reducedMotion),
        child: Scaffold(
          body: NeedIndicator(
            label: 'Сытость',
            value: value,
            color: GochyaColors.hunger,
          ),
        ),
      ),
    ),
  );
  await tester.pump();
}
