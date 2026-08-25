package reviewworkflow

import (
	"encoding/json"
	"fmt"
	"sort"
)

type UnitSpec struct {
	Paths        []string `json:"paths"`
	CoverageKeys []string `json:"coverageKeys"`
}

func NewUnitSpec(paths []string, coverageKeys []string) UnitSpec {
	return UnitSpec{
		Paths:        normalizedUnitSpecValues(paths),
		CoverageKeys: normalizedUnitSpecValues(coverageKeys),
	}
}

func (s UnitSpec) JSON() string {
	normalized := NewUnitSpec(s.Paths, s.CoverageKeys)
	encoded, _ := json.Marshal(normalized)
	return string(encoded)
}

func ParseUnitSpec(raw string) (UnitSpec, error) {
	spec := UnitSpec{}
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		return UnitSpec{}, fmt.Errorf("리뷰 unit spec을 읽지 못했습니다: %w", err)
	}
	return NewUnitSpec(spec.Paths, spec.CoverageKeys), nil
}

func normalizedUnitSpecValues(values []string) []string {
	normalized := append([]string(nil), values...)
	sort.Strings(normalized)
	unique := normalized[:0]
	for _, value := range normalized {
		if value == "" || len(unique) > 0 && unique[len(unique)-1] == value {
			continue
		}
		unique = append(unique, value)
	}
	return unique
}
