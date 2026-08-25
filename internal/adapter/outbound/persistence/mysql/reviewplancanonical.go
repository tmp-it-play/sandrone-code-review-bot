package mysql

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/mapper"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

func prepareInitialPlan(units []reviewworkflow.Unit, coverage []reviewworkflow.CoverageItem) ([]reviewworkflow.Unit, []reviewworkflow.CoverageItem, string, error) {
	preparedUnits := append([]reviewworkflow.Unit(nil), units...)
	preparedCoverage := append([]reviewworkflow.CoverageItem(nil), coverage...)
	unitHashes := make(map[string]struct{}, len(preparedUnits))
	orderKeys := make(map[string]struct{}, len(preparedUnits))
	unitPaths := make(map[string]map[string]struct{}, len(preparedUnits))
	pathOwners := make(map[string]string)
	coverageOwners := make(map[string]string)
	for index := range preparedUnits {
		unit := &preparedUnits[index]
		if len(unit.Hash) != 64 {
			return nil, nil, "", errors.New("unit hash 길이가 유효하지 않습니다")
		}
		if _, found := unitHashes[unit.Hash]; found {
			return nil, nil, "", fmt.Errorf("unit hash %s가 중복됩니다", unit.Hash)
		}
		unitHashes[unit.Hash] = struct{}{}
		if unit.ParentHash != "" || unit.Depth != 0 {
			return nil, nil, "", fmt.Errorf("초기 unit %s는 root여야 합니다", unit.Hash)
		}
		if unit.Ordinal <= 0 {
			return nil, nil, "", fmt.Errorf("초기 unit %s의 순서가 유효하지 않습니다", unit.Hash)
		}
		if unit.Kind == "" {
			return nil, nil, "", fmt.Errorf("초기 unit %s의 종류가 비어 있습니다", unit.Hash)
		}
		if unit.OrderKey == "" {
			unit.OrderKey = reviewworkflow.RootUnitOrderKey(unit.Ordinal)
		}
		if len(unit.OrderKey) > reviewworkflow.MaxUnitOrderKeyLength {
			return nil, nil, "", fmt.Errorf("unit %s의 order key가 너무 깁니다", unit.Hash)
		}
		if _, found := orderKeys[unit.OrderKey]; found {
			return nil, nil, "", fmt.Errorf("unit order key %s가 중복됩니다", unit.OrderKey)
		}
		orderKeys[unit.OrderKey] = struct{}{}
		spec, err := reviewworkflow.ParseUnitSpec(unit.SpecJSON)
		if err != nil {
			return nil, nil, "", fmt.Errorf("unit %s spec이 유효하지 않습니다: %w", unit.Hash, err)
		}
		if len(spec.Paths) == 0 || len(spec.CoverageKeys) == 0 {
			return nil, nil, "", fmt.Errorf("unit %s spec이 비어 있습니다", unit.Hash)
		}
		if unit.Hash != reviewworkflow.RootUnitHash(spec) {
			return nil, nil, "", fmt.Errorf("unit %s hash가 spec과 일치하지 않습니다", unit.Hash)
		}
		paths := make(map[string]struct{}, len(spec.Paths))
		for _, path := range spec.Paths {
			if owner, found := pathOwners[path]; found {
				return nil, nil, "", fmt.Errorf("파일 %s가 unit %s와 %s에 중복 배정되었습니다", path, owner, unit.Hash)
			}
			pathOwners[path] = unit.Hash
			paths[path] = struct{}{}
		}
		unitPaths[unit.Hash] = paths
		for _, coverageKey := range spec.CoverageKeys {
			if owner, found := coverageOwners[coverageKey]; found {
				return nil, nil, "", fmt.Errorf("coverage %s가 unit %s와 %s에 중복 배정되었습니다", coverageKey, owner, unit.Hash)
			}
			coverageOwners[coverageKey] = unit.Hash
		}
		unit.SpecJSON = spec.JSON()
		unit.Status = reviewworkflow.UnitStatusPending
		unit.ID = 0
		unit.RunID = 0
		unit.InputHash = ""
		unit.SplitHash = ""
		unit.AttemptCount = 0
		unit.Provider = ""
		unit.Model = ""
		unit.MultipleModels = false
		unit.PromptTokens = 0
		unit.CompletionTokens = 0
		unit.TotalTokens = 0
		unit.ToolExecutions = 0
		unit.ResultJSON = ""
		unit.Reused = false
		unit.Retryable = false
		unit.RetryAt = nil
		unit.ErrorSummary = ""
		unit.StartedAt = nil
		unit.FinishedAt = nil
		unit.HeartbeatAt = nil
		unit.LeaseToken = ""
		unit.LeaseExpiresAt = nil
	}
	coverageKeys := make(map[string]struct{}, len(preparedCoverage))
	for index := range preparedCoverage {
		item := &preparedCoverage[index]
		if len(item.Key) != 64 {
			return nil, nil, "", errors.New("coverage key 길이가 유효하지 않습니다")
		}
		if _, found := coverageKeys[item.Key]; found {
			return nil, nil, "", fmt.Errorf("coverage key %s가 중복됩니다", item.Key)
		}
		coverageKeys[item.Key] = struct{}{}
		if item.UnitHash != "" {
			if _, found := unitHashes[item.UnitHash]; !found {
				return nil, nil, "", fmt.Errorf("coverage %s의 unit %s를 찾지 못했습니다", item.Key, item.UnitHash)
			}
		}
		if item.Status == reviewworkflow.CoverageStatusPlanned && item.UnitHash == "" {
			return nil, nil, "", fmt.Errorf("계획된 coverage %s에 unit이 없습니다", item.Key)
		}
		if owner, found := coverageOwners[item.Key]; found {
			if _, pathFound := unitPaths[owner][item.Path]; !pathFound {
				return nil, nil, "", fmt.Errorf("coverage %s의 파일이 unit %s spec과 다릅니다", item.Key, owner)
			}
			if item.UnitHash != "" && item.UnitHash != owner {
				return nil, nil, "", fmt.Errorf("coverage %s의 unit 연결이 spec과 다릅니다", item.Key)
			}
		} else if item.UnitHash != "" {
			return nil, nil, "", fmt.Errorf("coverage %s가 unit spec에 없습니다", item.Key)
		}
		item.ID = 0
		item.RunID = 0
		item.UnitID = nil
		item.InitialUnitHash = item.UnitHash
		item.ReviewedAt = nil
	}
	for coverageKey := range coverageOwners {
		if _, found := coverageKeys[coverageKey]; !found {
			return nil, nil, "", fmt.Errorf("unit spec의 coverage %s를 계획에서 찾지 못했습니다", coverageKey)
		}
	}
	hash, err := initialPlanHash(preparedUnits, preparedCoverage)
	if err != nil {
		return nil, nil, "", err
	}
	return preparedUnits, preparedCoverage, hash, nil
}

func storedInitialPlanHash(unitEntries []model.ReviewUnit, coverageEntries []model.CoverageItem) (string, error) {
	units := make([]reviewworkflow.Unit, 0)
	for _, entry := range unitEntries {
		if entry.ParentUnitHash == "" {
			units = append(units, mapper.ToReviewUnit(entry))
		}
	}
	coverage := make([]reviewworkflow.CoverageItem, 0, len(coverageEntries))
	for _, entry := range coverageEntries {
		item := mapper.ToCoverageItem(entry)
		item.UnitHash = entry.InitialUnitHash
		coverage = append(coverage, item)
	}
	return initialPlanHash(units, coverage)
}

func initialPlanHash(units []reviewworkflow.Unit, coverage []reviewworkflow.CoverageItem) (string, error) {
	orderedUnits := append([]reviewworkflow.Unit(nil), units...)
	sort.Slice(orderedUnits, func(left int, right int) bool {
		return orderedUnits[left].Hash < orderedUnits[right].Hash
	})
	orderedCoverage := append([]reviewworkflow.CoverageItem(nil), coverage...)
	sort.Slice(orderedCoverage, func(left int, right int) bool {
		return orderedCoverage[left].Key < orderedCoverage[right].Key
	})
	hash := sha256.New()
	writePlanPart(hash, "adaptive-review-plan-v1")
	for _, unit := range orderedUnits {
		canonicalSpec, err := canonicalJSON(unit.SpecJSON)
		if err != nil {
			return "", err
		}
		writePlanPart(hash, "unit")
		writePlanPart(hash, unit.Hash)
		writePlanPart(hash, strconv.Itoa(unit.Ordinal))
		writePlanPart(hash, unit.OrderKey)
		writePlanPart(hash, unit.Kind)
		writePlanPart(hash, canonicalSpec)
	}
	for _, item := range orderedCoverage {
		initialUnitHash := item.InitialUnitHash
		if initialUnitHash == "" {
			initialUnitHash = item.UnitHash
		}
		writePlanPart(hash, "coverage")
		writePlanPart(hash, item.Key)
		writePlanPart(hash, initialUnitHash)
		writePlanPart(hash, string(item.Kind))
		writePlanPart(hash, item.Path)
		writePlanPart(hash, item.PreviousPath)
		writePlanPart(hash, item.FileStatus)
		writePlanPart(hash, item.HunkHash)
		writePlanPart(hash, strconv.Itoa(item.DuplicateOrdinal))
		writePlanPart(hash, strconv.Itoa(item.OldStart))
		writePlanPart(hash, strconv.Itoa(item.OldCount))
		writePlanPart(hash, strconv.Itoa(item.NewStart))
		writePlanPart(hash, strconv.Itoa(item.NewCount))
		writePlanPart(hash, string(item.Eligibility))
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func canonicalJSON(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", errors.New("spec JSON이 비어 있습니다")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return "", errors.New("spec JSON 뒤에 추가 값이 있습니다")
		}
		return "", err
	}
	buffer := bytes.Buffer{}
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return "", err
	}
	return strings.TrimSpace(buffer.String()), nil
}

func writePlanPart(writer interface{ Write([]byte) (int, error) }, value string) {
	writer.Write([]byte(strconv.Itoa(len(value))))
	writer.Write([]byte{0})
	writer.Write([]byte(value))
}
