package reviewworkflow

const RootUnitStrategyVersion = "semantic-review-unit-v1"

func RootUnitHash(spec UnitSpec) string {
	normalized := NewUnitSpec(spec.Paths, spec.CoverageKeys)
	return hashParts(append([]string{RootUnitStrategyVersion}, normalized.CoverageKeys...)...)
}
