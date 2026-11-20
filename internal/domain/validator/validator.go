package validator

import "trippilot/internal/domain"

// BaseRuleCount is the number of deterministic checks C1-C12.
const BaseRuleCount = 12

// CheckedCount reports how many rules ran, including the optional rain check.
func CheckedCount(includesRain bool) int {
	if includesRain {
		return BaseRuleCount + 1
	}
	return BaseRuleCount
}

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
	violations = append(violations, checkC11(plan, pois, constraints)...)
	violations = append(violations, checkC12(plan, pois)...)
	domain.Sort(violations)
	return violations
}
