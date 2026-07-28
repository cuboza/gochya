---
name: gochya-visual
description: Visual rules for GOCHYA screens — colour scales, motion, accessibility and the checks that catch drift. Load before building or changing any companion UI.
---

# GOCHYA visual system

The source of truth is `docs/06-art/ART_BIBLE.md` and `docs/06-art/UX_UI.md`.
This is the short form: the rules that get broken most often, and the checks
that catch them.

## Colour: five scales that must not borrow from each other

| Scale | Where | Class |
|---|---|---|
| Base UI | фон, CTA, Koins, успех, ошибка | `GochyaColors` |
| Потребности | голод, энергия, гигиена, настроение | `GochyaColors.hunger` … |
| Стихии | вид существа | `GochyaElementColors` |
| Редкости | рамки карт | `GochyaRarityColors` |
| Кольца активности | шаги, сон, калории | `GochyaRingColors` |

Every audit this project ever ran was about one scale leaking into another.
Water once took the `energy` colour, so an Earth pet was tinted mint above a
mint hygiene bar. Low mood in `warning` pink was indistinguishable from an
error. **Never reach across scales.** `element_palette_test.dart` fails loudly
if you do.

New colour needed? Check `ART_BIBLE.md` §3 first — the palette is crowded, and
every hue is claimed. Activity rings ended up as one hue at three lightness
steps for exactly this reason.

## Motion is never the only channel

Anything a movement says must also be readable when it stops:

- Reduce motion kills the reaction outright — do not rely on Flutter shortening
  the controller to 5%, the animation still runs and its particles still flash.
- A low need pulses **and** carries a static «мало» marker.
- No blinking. WCAG 2.3.1 rules out flashes; use a slow pulse.

## Never invent data

No health source connected means empty rings and an honest line, not a
zero-scoring day. No endpoint for league or friends means those rows are not
drawn at all — an empty «Лига —» promises what the backend cannot deliver.

## Simplicity beats completeness on the main screen

Progressive disclosure: home carries what a player returns to daily, the rest
sits one tap away. Type-level flavour text (a technique's description and lore)
repeats on every card of that type in a list — keep it out of collections.
Copy describing how the client works is not copy for the player.

## Checks that catch drift

```bash
bash tools/build-core-host.sh        # FFI tests need the real library
cd clients/companion
dart format lib test && dart analyze lib test
flutter test --no-pub
```

Three of these tests exist because a bug shipped past review:

- `text_scaling_test.dart` — 200% type on a **390 px** canvas. Use a real phone
  width; on a wide canvas nothing overflows and the test proves nothing.
- `accessibility_test.dart` — tap targets, labels, contrast on all five tabs.
  It found a 39 px overflow at *normal* type that the 411 px emulator hid.
- `element_palette_test.dart` — the scale-collision guard above.

When you add a layout test, break the code on purpose once and confirm the test
fails. Two tests in this repo were silently vacuous until that was done.
