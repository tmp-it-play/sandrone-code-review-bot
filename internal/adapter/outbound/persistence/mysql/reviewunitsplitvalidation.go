package mysql

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/mapper"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func validateUnitSplitInput(split reviewworkflow.UnitSplit) error {
	if split.ParentHash == "" || split.ParentLeaseToken == "" || split.ParentInputHash == "" {
		return errors.New("부모 unit 식별자, lease, input hash가 필요합니다")
	}
	if split.ParentResult.Status != reviewworkflow.UnitStatusFailed {
		return errors.New("실패 결과만 분할할 수 있습니다")
	}
	if split.ParentResult.InputHash != "" && split.ParentResult.InputHash != split.ParentInputHash {
		return errors.New("부모 unit input hash가 실패 결과와 다릅니다")
	}
	if split.ParentResult.FinishedAt.IsZero() || split.RefinedAt.IsZero() {
		return errors.New("분할 완료 시각이 필요합니다")
	}
	if len(split.Children) != 2 {
		return errors.New("unit은 정확히 두 child로 분할해야 합니다")
	}
	if len(split.Assignments) == 0 {
		return errors.New("coverage 재배정이 비어 있습니다")
	}
	return nil
}

func prepareSplitChildren(parent model.ReviewUnit, requested []reviewworkflow.Unit) ([]reviewworkflow.Unit, error) {
	children := append([]reviewworkflow.Unit(nil), requested...)
	hashes := make(map[string]struct{}, len(children))
	childSpecs := make([]reviewworkflow.UnitSpec, 0, len(children))
	for index := range children {
		child := &children[index]
		if len(child.Hash) != 64 || child.Hash == parent.UnitHash {
			return nil, errors.New("child unit hash가 유효하지 않습니다")
		}
		if _, found := hashes[child.Hash]; found {
			return nil, fmt.Errorf("child unit hash %s가 중복됩니다", child.Hash)
		}
		hashes[child.Hash] = struct{}{}
		if child.ParentHash == "" {
			child.ParentHash = parent.UnitHash
		}
		if child.ParentHash != parent.UnitHash {
			return nil, fmt.Errorf("child unit %s의 부모가 다릅니다", child.Hash)
		}
		expectedDepth := parent.Depth + 1
		if child.Depth == 0 {
			child.Depth = expectedDepth
		}
		if child.Depth != expectedDepth {
			return nil, fmt.Errorf("child unit %s의 깊이가 유효하지 않습니다", child.Hash)
		}
		expectedOrderKey := reviewworkflow.ChildUnitOrderKey(parent.OrderKey, index)
		if child.OrderKey == "" {
			child.OrderKey = expectedOrderKey
		}
		if child.OrderKey != expectedOrderKey {
			return nil, fmt.Errorf("child unit %s의 순서가 유효하지 않습니다", child.Hash)
		}
		if len(child.OrderKey) > reviewworkflow.MaxUnitOrderKeyLength {
			return nil, fmt.Errorf("child unit %s의 order key가 너무 깁니다", child.Hash)
		}
		if child.Ordinal == 0 {
			child.Ordinal = parent.Ordinal
		}
		if child.Kind == "" {
			child.Kind = parent.Kind
		}
		canonicalSpec, err := canonicalJSON(child.SpecJSON)
		if err != nil {
			return nil, fmt.Errorf("child unit %s spec이 유효하지 않습니다: %w", child.Hash, err)
		}
		spec, err := reviewworkflow.ParseUnitSpec(canonicalSpec)
		if err != nil {
			return nil, fmt.Errorf("child unit %s spec을 읽지 못했습니다: %w", child.Hash, err)
		}
		if len(spec.Paths) == 0 || len(spec.CoverageKeys) == 0 {
			return nil, fmt.Errorf("child unit %s spec이 비어 있습니다", child.Hash)
		}
		childSpecs = append(childSpecs, spec)
		child.SpecJSON = spec.JSON()
		if child.Hash != reviewworkflow.AdaptiveUnitHash(spec) {
			return nil, fmt.Errorf("child unit %s hash가 spec과 일치하지 않습니다", child.Hash)
		}
		if child.Status == "" {
			child.Status = reviewworkflow.UnitStatusPending
		}
		if child.Status != reviewworkflow.UnitStatusPending {
			return nil, fmt.Errorf("child unit %s는 pending 상태여야 합니다", child.Hash)
		}
		child.ID = 0
		child.RunID = 0
		child.InputHash = ""
		child.SplitHash = ""
		child.AttemptCount = 0
		child.Provider = ""
		child.Model = ""
		child.MultipleModels = false
		child.PromptTokens = 0
		child.CompletionTokens = 0
		child.TotalTokens = 0
		child.ToolExecutions = 0
		child.ResultJSON = ""
		child.Reused = false
		child.Retryable = false
		child.RetryAt = nil
		child.ErrorSummary = ""
		child.StartedAt = nil
		child.FinishedAt = nil
		child.HeartbeatAt = nil
		child.LeaseToken = ""
		child.LeaseExpiresAt = nil
	}
	if err := validateChildPathPartition(parent.SpecJSON, childSpecs); err != nil {
		return nil, err
	}
	return children, nil
}

func validateChildPathPartition(parentSpecJSON string, children []reviewworkflow.UnitSpec) error {
	parentSpec, err := reviewworkflow.ParseUnitSpec(parentSpecJSON)
	if err != nil {
		return fmt.Errorf("부모 unit spec을 읽지 못했습니다: %w", err)
	}
	childPaths := make([]string, 0, len(parentSpec.Paths))
	seen := make(map[string]struct{}, len(parentSpec.Paths))
	for _, child := range children {
		for _, path := range child.Paths {
			if _, found := seen[path]; found {
				return fmt.Errorf("파일 %s가 여러 child unit에 배정되었습니다", path)
			}
			seen[path] = struct{}{}
			childPaths = append(childPaths, path)
		}
	}
	sort.Strings(childPaths)
	if !equalStrings(childPaths, parentSpec.Paths) {
		return errors.New("부모 unit의 파일 전체가 child에 배정되지 않았습니다")
	}
	return nil
}

func validateRunningSplitParent(transaction *gorm.DB, parent model.ReviewUnit, split reviewworkflow.UnitSplit) error {
	if reviewworkflow.UnitStatus(parent.Status) != reviewworkflow.UnitStatusRunning || parent.LeaseToken != split.ParentLeaseToken || parent.LeaseExpiresAt == nil {
		return errors.New("실행 중인 leaf unit만 분할할 수 있습니다")
	}
	if parent.InputHash != split.ParentInputHash {
		return errors.New("분할할 unit의 input hash가 변경되었습니다")
	}
	now, err := databaseTime(transaction)
	if err != nil {
		return err
	}
	if !parent.LeaseExpiresAt.After(now) {
		return errors.New("분할할 unit의 lease가 만료되었습니다")
	}
	var childCount int64
	if err := transaction.Model(&model.ReviewUnit{}).Where("review_run_id = ? AND parent_unit_hash = ?", parent.ReviewRunID, parent.UnitHash).Count(&childCount).Error; err != nil {
		return err
	}
	if childCount != 0 {
		return errors.New("분할되지 않은 부모 unit에 child가 이미 있습니다")
	}
	return nil
}

func validateSplitAssignments(transaction *gorm.DB, runID uint64, parent model.ReviewUnit, children []reviewworkflow.Unit, assignments []reviewworkflow.CoverageAssignment) (map[string][]uint64, error) {
	var coverageEntries []model.CoverageItem
	if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("review_run_id = ? AND review_unit_id = ? AND status = ?", runID, parent.ID, string(reviewworkflow.CoverageStatusPlanned)).
		Order("id ASC").Find(&coverageEntries).Error; err != nil {
		return nil, err
	}
	if len(coverageEntries) == 0 {
		return nil, errors.New("분할할 planned coverage가 없습니다")
	}
	childHashes := map[string]struct{}{children[0].Hash: {}, children[1].Hash: {}}
	coverageByKey := make(map[string]model.CoverageItem, len(coverageEntries))
	for _, entry := range coverageEntries {
		coverageByKey[entry.CoverageKey] = entry
	}
	assigned := make(map[string]struct{}, len(assignments))
	assignmentIDs := map[string][]uint64{children[0].Hash: {}, children[1].Hash: {}}
	for _, assignment := range assignments {
		if _, found := childHashes[assignment.UnitHash]; !found {
			return nil, fmt.Errorf("coverage %s의 child unit이 유효하지 않습니다", assignment.CoverageKey)
		}
		entry, found := coverageByKey[assignment.CoverageKey]
		if !found {
			return nil, fmt.Errorf("coverage %s는 부모의 planned 항목이 아닙니다", assignment.CoverageKey)
		}
		if _, found := assigned[assignment.CoverageKey]; found {
			return nil, fmt.Errorf("coverage %s가 중복 배정되었습니다", assignment.CoverageKey)
		}
		assigned[assignment.CoverageKey] = struct{}{}
		assignmentIDs[assignment.UnitHash] = append(assignmentIDs[assignment.UnitHash], entry.ID)
	}
	if len(assigned) != len(coverageEntries) {
		return nil, errors.New("부모의 planned coverage 전체가 child에 배정되지 않았습니다")
	}
	for _, child := range children {
		if len(assignmentIDs[child.Hash]) == 0 {
			return nil, fmt.Errorf("child unit %s의 coverage가 비어 있습니다", child.Hash)
		}
		spec, err := reviewworkflow.ParseUnitSpec(child.SpecJSON)
		if err != nil {
			return nil, err
		}
		paths := make(map[string]struct{}, len(spec.Paths))
		for _, path := range spec.Paths {
			paths[path] = struct{}{}
		}
		assignedKeys := make([]string, 0, len(assignmentIDs[child.Hash]))
		for _, coverageID := range assignmentIDs[child.Hash] {
			for _, entry := range coverageEntries {
				if entry.ID == coverageID {
					if _, found := paths[entry.Path]; !found {
						return nil, fmt.Errorf("coverage %s의 파일이 child unit %s spec에 없습니다", entry.CoverageKey, child.Hash)
					}
					assignedKeys = append(assignedKeys, entry.CoverageKey)
					break
				}
			}
		}
		sort.Strings(assignedKeys)
		if !equalStrings(assignedKeys, spec.CoverageKeys) {
			return nil, fmt.Errorf("child unit %s spec과 coverage 배정이 다릅니다", child.Hash)
		}
	}
	return assignmentIDs, nil
}

func validateStoredUnitSplit(transaction *gorm.DB, runID uint64, parent model.ReviewUnit, expectedChildren []reviewworkflow.Unit, assignments []reviewworkflow.CoverageAssignment, splitHash string, parentInputHash string) error {
	if parent.InputHash != parentInputHash || parent.SplitHash != splitHash {
		return errors.New("저장된 unit 분할 요청이 현재 요청과 다릅니다")
	}
	var directChildren []model.ReviewUnit
	if err := transaction.Where("review_run_id = ? AND parent_unit_hash = ?", runID, parent.UnitHash).Order("order_key ASC").Find(&directChildren).Error; err != nil {
		return err
	}
	if len(directChildren) != len(expectedChildren) {
		return errors.New("저장된 child unit 수가 분할 요청과 다릅니다")
	}
	for index, entry := range directChildren {
		expected := expectedChildren[index]
		stored := mapper.ToReviewUnit(entry)
		if stored.Hash != expected.Hash || stored.ParentHash != expected.ParentHash || stored.Depth != expected.Depth || stored.OrderKey != expected.OrderKey || stored.Kind != expected.Kind || stored.SpecJSON != expected.SpecJSON {
			return errors.New("저장된 child unit이 분할 요청과 다릅니다")
		}
	}
	return validateStoredCoverageAssignments(transaction, runID, parent.UnitHash, assignments)
}

func validateStoredCoverageAssignments(transaction *gorm.DB, runID uint64, parentHash string, assignments []reviewworkflow.CoverageAssignment) error {
	var units []model.ReviewUnit
	if err := transaction.Where("review_run_id = ?", runID).Find(&units).Error; err != nil {
		return err
	}
	unitByID := make(map[uint64]model.ReviewUnit, len(units))
	unitByHash := make(map[string]model.ReviewUnit, len(units))
	for _, unit := range units {
		unitByID[unit.ID] = unit
		unitByHash[unit.UnitHash] = unit
	}
	requested := make(map[string]string, len(assignments))
	keys := make([]string, 0, len(assignments))
	for _, assignment := range assignments {
		if _, found := requested[assignment.CoverageKey]; found {
			return fmt.Errorf("coverage %s가 중복 배정되었습니다", assignment.CoverageKey)
		}
		requested[assignment.CoverageKey] = assignment.UnitHash
		keys = append(keys, assignment.CoverageKey)
	}
	var coverage []model.CoverageItem
	if err := transaction.Where("review_run_id = ? AND coverage_key IN ?", runID, keys).Find(&coverage).Error; err != nil {
		return err
	}
	if len(coverage) != len(requested) {
		return errors.New("저장된 coverage 재배정이 분할 요청과 다릅니다")
	}
	for _, item := range coverage {
		if item.ReviewUnitID == nil {
			return errors.New("분할된 coverage의 leaf unit 연결이 없습니다")
		}
		current, found := unitByID[*item.ReviewUnitID]
		if !found {
			return errors.New("분할된 coverage의 leaf unit을 찾지 못했습니다")
		}
		expectedChild := requested[item.CoverageKey]
		if !unitDescendsFrom(current, expectedChild, unitByHash) || expectedChild == parentHash {
			return errors.New("저장된 coverage child 연결이 분할 요청과 다릅니다")
		}
	}
	return nil
}

func unitDescendsFrom(unit model.ReviewUnit, ancestorHash string, units map[string]model.ReviewUnit) bool {
	current := unit
	visited := make(map[string]struct{})
	for current.UnitHash != "" {
		if _, found := visited[current.UnitHash]; found {
			return false
		}
		visited[current.UnitHash] = struct{}{}
		if current.UnitHash == ancestorHash {
			return true
		}
		if current.ParentUnitHash == "" {
			return false
		}
		parent, found := units[current.ParentUnitHash]
		if !found {
			return false
		}
		current = parent
	}
	return false
}

func splitAssignmentShape(assignments []reviewworkflow.CoverageAssignment) []string {
	shape := make([]string, 0, len(assignments))
	for _, assignment := range assignments {
		shape = append(shape, assignment.CoverageKey+"\x00"+assignment.UnitHash)
	}
	sort.Strings(shape)
	return shape
}

func unitSplitHash(parentInputHash string, children []reviewworkflow.Unit, assignments []reviewworkflow.CoverageAssignment) string {
	hash := sha256.New()
	writePlanPart(hash, "adaptive-unit-split-v1")
	writePlanPart(hash, parentInputHash)
	orderedChildren := append([]reviewworkflow.Unit(nil), children...)
	sort.Slice(orderedChildren, func(left int, right int) bool {
		return orderedChildren[left].Hash < orderedChildren[right].Hash
	})
	for _, child := range orderedChildren {
		writePlanPart(hash, child.Hash)
		writePlanPart(hash, child.ParentHash)
		writePlanPart(hash, fmt.Sprintf("%d", child.Depth))
		writePlanPart(hash, child.OrderKey)
		writePlanPart(hash, child.Kind)
		writePlanPart(hash, child.SpecJSON)
	}
	for _, assignment := range splitAssignmentShape(assignments) {
		writePlanPart(hash, assignment)
	}
	return hex.EncodeToString(hash.Sum(nil))
}
