package reviewworkflow

import "strconv"

const AdaptiveUnitStrategyVersion = "adaptive-review-leaf-v1"

func AdaptiveUnitHash(spec UnitSpec) string {
	normalized := NewUnitSpec(spec.Paths, spec.CoverageKeys)
	parts := []string{AdaptiveUnitStrategyVersion, strconv.Itoa(len(normalized.Paths))}
	parts = append(parts, normalized.Paths...)
	parts = append(parts, strconv.Itoa(len(normalized.CoverageKeys)))
	parts = append(parts, normalized.CoverageKeys...)
	return hashParts(parts...)
}
