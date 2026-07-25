import 'profile_models.dart';

/// `BreedCostKoins` from `server/internal/breeding/model.go`.
const breedCostKoins = 500;

/// Eligibility gate enforced by the server before an egg is created.
const breedMinimumParentLevel = 30;

enum BreedingCatalyst {
  mutation('mutation', 'Катализатор мутации'),
  hybrid('hybrid', 'Катализатор гибрида');

  const BreedingCatalyst(this.apiValue, this.label);

  final String apiValue;
  final String label;
}

class BreedingResult {
  const BreedingResult({required this.eggId, required this.incubateUntil});

  factory BreedingResult.fromJson(JsonMap json) {
    return BreedingResult(
      eggId: requiredString(json, 'eggId'),
      incubateUntil: requiredDateTime(json, 'incubateUntil'),
    );
  }

  final String eggId;
  final DateTime incubateUntil;
}

/// Local eligibility mirror used only to disable impossible requests early.
/// The server stays the authority and re-checks every rule.
bool canBreed(PetSummary pet) {
  return pet.stage.toLowerCase() == 'adult' &&
      pet.level >= breedMinimumParentLevel &&
      !pet.isWeak;
}
