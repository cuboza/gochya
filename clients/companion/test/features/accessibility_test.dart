import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gochya_companion/app/app.dart';
import 'package:gochya_companion/dev/demo_mode.dart';

/// Holds the accessibility line `UX_UI.md` §10 and `ART_BIBLE.md` §6.1 draw,
/// using Flutter's own guideline matchers rather than prose:
///
/// * tap targets big enough to hit (§6.1, WCAG 2.2 SC 2.5.8),
/// * every tap target labelled, or TalkBack announces "button, button",
/// * text contrast at WCAG AA.
///
/// Demo mode fakes every repository, so one walk covers all five tabs.
void main() {
  testWidgets('every tab meets the accessibility guidelines', (tester) async {
    await tester.binding.setSurfaceSize(const Size(390, 2400));
    addTearDown(() => tester.binding.setSurfaceSize(null));
    final handle = tester.ensureSemantics();

    await tester.pumpWidget(const DemoPlayerScope(child: GochyaApp()));
    await tester.pumpAndSettle();

    for (final tab in ['Главная', 'Магазин', 'PvP', 'Бридинг', 'Профиль']) {
      await tester.tap(find.text(tab));
      await tester.pumpAndSettle();

      await expectLater(tester, meetsGuideline(androidTapTargetGuideline));
      await expectLater(tester, meetsGuideline(iOSTapTargetGuideline));
      await expectLater(tester, meetsGuideline(labeledTapTargetGuideline));
      await expectLater(tester, meetsGuideline(textContrastGuideline));
    }

    handle.dispose();
  });
}
