package validator

import "trippilot/internal/domain"

// Validate runs deterministic checks and returns violations in stable order.
func Validate(
	plan domain.Itinerary,
	constraints domain.ConstraintSet,
	pois map[string]domain.POI,
	routes domain.RouteProvider,
) []domain.Violation {
	violations := make([]domain.Violation, 0)
	violations = append(violations, checkOpenHours(plan, pois)...)
	domain.Sort(violations)
	return violations
}
