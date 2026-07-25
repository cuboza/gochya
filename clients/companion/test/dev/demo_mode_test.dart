import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gochya_companion/app/app.dart';
import 'package:gochya_companion/dev/demo_mode.dart';

void main() {
  testWidgets('demo mode opens a populated signed-in player flow', (
    tester,
  ) async {
    await tester.pumpWidget(const DemoPlayerScope(child: GochyaApp()));
    await tester.pumpAndSettle();

    expect(find.text('Привет, Ника'), findsOneWidget);
    expect(find.text('Моти'), findsOneWidget);
    expect(find.text('81%'), findsOneWidget);

    await tester.scrollUntilVisible(
      find.byKey(const Key('care-feed')),
      300,
      scrollable: find.byType(Scrollable).first,
    );
    await tester.tap(find.byKey(const Key('care-feed')));
    await tester.pumpAndSettle();

    expect(find.text('Питомец накормлен.'), findsOneWidget);
    await tester.fling(
      find.byType(Scrollable).first,
      const Offset(0, 1000),
      1000,
    );
    await tester.pumpAndSettle();
    expect(find.text('81%'), findsNothing);
    expect(find.text('93%'), findsOneWidget);

    await tester.tap(find.text('Магазин'));
    await tester.pumpAndSettle();

    expect(find.text('500 Koins'), findsOneWidget);
    expect(find.text('Яблоко'), findsOneWidget);
    expect(find.textContaining('В наличии: 3'), findsOneWidget);

    await tester.tap(find.widgetWithText(FilledButton, '20 K'));
    await tester.pumpAndSettle();

    expect(find.text('480 Koins'), findsOneWidget);
    expect(find.textContaining('В наличии: 4'), findsOneWidget);
  });
}
