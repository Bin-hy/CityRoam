package validator

import "trippilot/internal/domain"

// Validate runs deterministic checks and returns violations in stable order.
func Validate(
	plan domain.Itinerary,
	constraints domain.ConstraintSet,
	pois map[string]domain.POI,
	routes domain.RouteProvider,
) []domain.Violation {
	constraints = constraints.ApplyDefaults()

	violations := make([]domain.Violation, 0)
	violations = append(violations, checkOpenHours(plan, pois)...)
	violations = append(violations, checkC3(plan, pois, routes)...)
	violations = append(violations, checkC4(plan, constraints)...)
	violations = append(violations, checkC5(plan, constraints)...)
	violations = append(violations, checkC6(plan, constraints)...)
	violations = append(violations, checkC7(plan, pois, constraints)...)
	violations = append(violations, checkC8(plan)...)
	violations = append(violations, checkC9(plan, constraints)...)
	violations = append(violations, checkC10(plan, constraints)...)
	violations = append(violations, checkC12(plan, pois)...)
	domain.Sort(violations)
	return violations
}
