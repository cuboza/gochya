import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gochya_companion/app/theme.dart';
import 'package:gochya_companion/features/dojo_upsell/dojo_upsell_screen.dart';

void main() {
  testWidgets('names both routes to cards and their ceilings', (tester) async {
    await _pump(tester);

    expect(find.text('Игровая добыча'), findsOneWidget);
    expect(find.text('до Epic'), findsOneWidget);
    expect(find.text('Запись удара'), findsOneWidget);
    expect(find.text('до Mythic'), findsOneWidget);
  });

  testWidgets('says the phone player is already competitive', (tester) async {
    await _pump(tester);

    // `MECHANIC_COMBAT_RECORDING.md` §0: recording buys quality and identity,
    // never a monopoly on strength. Dropping this reassurance would turn the
    // screen into the "buy a watch or lose" pitch the spec forbids.
    expect(find.text('Ты уже конкурентоспособен'), findsOneWidget);
    expect(find.textContaining('хватает для любой лиги'), findsOneWidget);
  });

  testWidgets('blocks nothing and promises no purchase', (tester) async {
    await _pump(tester);

    // No disabled control may appear here: a greyed-out button would read as
    // a locked feature, which is exactly the feeling the spec rules out.
    final buttons = tester.widgetList<ButtonStyleButton>(
      find.byType(ButtonStyleButton),
    );
    for (final button in buttons) {
      expect(button.onPressed, isNotNull);
    }

    for (final forbidden in ['Купить', 'Осталось', 'Недоступно', 'Заблок']) {
      expect(
        find.textContaining(forbidden),
        findsNothing,
        reason: 'the screen must not pressure the player with "$forbidden"',
      );
    }
  });

  testWidgets('offers no connect action while pairing does not exist', (
    tester,
  ) async {
    await _pump(tester);

    // Honest by omission: there is no watch pairing in this client, so the
    // card explains what to do instead of showing a button that does nothing.
    expect(
      find.textContaining('Есть Galaxy Watch или Apple Watch?'),
      findsOneWidget,
    );
    expect(find.widgetWithText(FilledButton, 'Подключить'), findsNothing);
  });

  testWidgets('explains why recording is watch-only', (tester) async {
    await _pump(tester);

    expect(find.textContaining('акселерометр на запястье'), findsOneWidget);
  });
}

Future<void> _pump(WidgetTester tester) async {
  await tester.binding.setSurfaceSize(const Size(1000, 2600));
  addTearDown(() => tester.binding.setSurfaceSize(null));
  await tester.pumpWidget(
    MaterialApp(theme: buildGochyaTheme(), home: const DojoUpsellScreen()),
  );
  await tester.pumpAndSettle();
}
