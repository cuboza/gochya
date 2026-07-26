import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gochya_companion/app/gochya_loader.dart';
import 'package:gochya_companion/app/theme.dart';

void main() {
  testWidgets('says what is being waited for', (tester) async {
    await _pump(tester, caption: 'Ищем соперника…');

    expect(find.text('Ищем соперника…'), findsOneWidget);
  });

  testWidgets('announces the wait to a screen reader', (tester) async {
    final handle = tester.ensureSemantics();
    await _pump(tester, caption: 'Считаем Vitality…');

    expect(find.bySemanticsLabel('Считаем Vitality…'), findsOneWidget);
    handle.dispose();
  });

  testWidgets('falls back to a generic label with no caption', (tester) async {
    final handle = tester.ensureSemantics();
    await _pump(tester);

    expect(find.bySemanticsLabel('Загрузка'), findsOneWidget);
    handle.dispose();
  });

  testWidgets('keeps the dots visible under reduced motion', (tester) async {
    // Reduced motion must not turn the loader into an empty screen: it still
    // has to say "something is coming".
    await _pump(tester, caption: 'Будим питомца…');
    await tester.pump(const Duration(milliseconds: 400));

    expect(find.byType(GochyaLoader), findsOneWidget);
    expect(find.text('Будим питомца…'), findsOneWidget);
    expect(_dotCount(tester), 4);
  });

  testWidgets('animates the dots when motion is allowed', (tester) async {
    await _pump(tester, caption: 'Будим питомца…', reducedMotion: false);

    final first = _dotSizes(tester);
    await tester.pump(const Duration(milliseconds: 350));
    final second = _dotSizes(tester);

    expect(first, isNot(second));
    // Never collapses to nothing — a dot that vanishes reads as a glitch.
    for (final size in [...first, ...second]) {
      expect(size, greaterThan(0));
    }
  });
}

int _dotCount(WidgetTester tester) {
  return tester
      .widgetList<Container>(
        find.descendant(
          of: find.byType(GochyaLoader),
          matching: find.byType(Container),
        ),
      )
      .length;
}

List<double> _dotSizes(WidgetTester tester) {
  return tester
      .widgetList<Container>(
        find.descendant(
          of: find.byType(GochyaLoader),
          matching: find.byType(Container),
        ),
      )
      .map((container) => (container.constraints?.maxWidth ?? 0))
      .toList();
}

Future<void> _pump(
  WidgetTester tester, {
  String? caption,
  bool reducedMotion = true,
}) async {
  await tester.pumpWidget(
    MaterialApp(
      theme: buildGochyaTheme(),
      home: MediaQuery(
        data: MediaQueryData(disableAnimations: reducedMotion),
        child: Scaffold(body: GochyaLoader(caption: caption)),
      ),
    ),
  );
  await tester.pump();
}
