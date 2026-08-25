package reviewworkflow

import (
	"errors"
	"fmt"
)

func NewChildUnit(parent Unit, index int, spec UnitSpec) (Unit, error) {
	if parent.Hash == "" || parent.OrderKey == "" {
		return Unit{}, errors.New("부모 unit 식별자와 순서가 필요합니다")
	}
	if index < 0 || index > 1 {
		return Unit{}, fmt.Errorf("child unit 순서 %d가 유효하지 않습니다", index)
	}
	normalized := NewUnitSpec(spec.Paths, spec.CoverageKeys)
	if len(normalized.Paths) == 0 || len(normalized.CoverageKeys) == 0 {
		return Unit{}, errors.New("child unit spec이 비어 있습니다")
	}
	return Unit{
		Hash:       AdaptiveUnitHash(normalized),
		ParentHash: parent.Hash,
		Ordinal:    parent.Ordinal,
		Depth:      parent.Depth + 1,
		OrderKey:   ChildUnitOrderKey(parent.OrderKey, index),
		Kind:       parent.Kind,
		SpecJSON:   normalized.JSON(),
		Status:     UnitStatusPending,
	}, nil
}
