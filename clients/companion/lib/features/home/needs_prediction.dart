import '../../core/ffi/gochya_core.dart';
import '../../core/models/profile_models.dart';

/// Predicts how far a pet's needs have decayed since the server last spoke.
///
/// The server stays authoritative: this only fills the gap between profile
/// reads so a pet left open on screen does not look frozen. Every number comes
/// out of the Shared Core, which owns the decay curve — nothing here does
/// arithmetic of its own.
///
/// Anything the Core refuses falls back to the server's value rather than a
/// locally invented one. In particular the Core will not advance more than 24
/// hours, so a pet the player has not opened since yesterday simply shows what
/// the server sent, which is correct: after that long the server's value is the
/// only thing that can be right.
PetNeeds predictNeeds({
  required GochyaCore? core,
  required PetSummary pet,
  required DateTime now,
}) {
  if (core == null) {
    return pet.needs;
  }
  final elapsed = now.difference(pet.needsUpdatedAt);
  if (elapsed <= Duration.zero) {
    return pet.needs;
  }
  final sleepingUntil = pet.sleepingUntil;
  try {
    final advanced = core.advanceNeeds(
      CoreNeedsState(
        hunger: pet.needs.hunger,
        energy: pet.needs.energy,
        hygiene: pet.needs.hygiene,
        mood: pet.needs.mood,
        isSleeping: sleepingUntil != null && sleepingUntil.isAfter(now),
        isWeak: pet.isWeak,
      ),
      elapsed,
    );
    return PetNeeds(
      hunger: advanced.hunger,
      energy: advanced.energy,
      hygiene: advanced.hygiene,
      mood: advanced.mood,
    );
  } on CoreException {
    return pet.needs;
  }
}
