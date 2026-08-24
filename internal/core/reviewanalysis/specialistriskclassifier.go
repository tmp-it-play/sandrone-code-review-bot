package reviewanalysis

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
)

type SpecialistRiskClassifier struct{}

func (c SpecialistRiskClassifier) Guidance(files []pullrequest.ChangedFile) string {
	risks := c.Classify(files)
	if len(risks) == 0 {
		return ""
	}
	return fmt.Sprintf("변경 경로에서 감지한 전문 검토 관점: %s. 별도 호출 없이 같은 리뷰에서 해당 관점의 실패 조건만 확인한다.\n\n", strings.Join(risks, "; "))
}

func (c SpecialistRiskClassifier) Classify(files []pullrequest.ChangedFile) []string {
	detected := map[string]struct{}{}
	for _, file := range files {
		path := strings.ToLower(filepath.ToSlash(file.Path))
		extension := strings.ToLower(filepath.Ext(path))
		if containsAny(path, "auth", "security", "crypto", "secret", "permission", "policy", "session", "token") {
			detected["보안·권한 경계"] = struct{}{}
		}
		if containsAny(path, "migration", "schema", "database", "persistence", "repository", "/db/", "/sql/") || extension == ".sql" {
			detected["데이터 무결성·마이그레이션"] = struct{}{}
		}
		if containsAny(path, "queue", "worker", "job", "async", "concurr", "cache", "lock", "lease", "scheduler") {
			detected["동시성·재시도·멱등성"] = struct{}{}
		}
		if containsAny(path, "handler", "controller", "router", "openapi", "graphql", "/api/", "/http/") || extension == ".proto" {
			detected["API 계약·호환성"] = struct{}{}
		}
		if containsAny(path, ".github/workflows", "dockerfile", "docker-compose", "/deploy/", "terraform", "kubernetes", "go.mod", "go.sum", "package.json", "package-lock.json", "pnpm-lock", "yarn.lock") {
			detected["배포·공급망·롤백"] = struct{}{}
		}
		if file.IsRemoved() {
			detected["삭제 영향·하위 호환성"] = struct{}{}
		}
		if file.PreviousPath != "" || (file.Deletions > 20 && file.Deletions > file.Additions*2) {
			detected["삭제 영향·하위 호환성"] = struct{}{}
		}
		if file.Truncated || file.PatchTruncated {
			detected["불완전 diff·주변 문맥"] = struct{}{}
		}
	}
	priority := []string{
		"보안·권한 경계",
		"데이터 무결성·마이그레이션",
		"동시성·재시도·멱등성",
		"API 계약·호환성",
		"배포·공급망·롤백",
		"불완전 diff·주변 문맥",
		"삭제 영향·하위 호환성",
	}
	ordered := make([]string, 0, len(detected))
	for _, risk := range priority {
		if _, ok := detected[risk]; ok {
			ordered = append(ordered, risk)
		}
	}
	if len(ordered) > 4 {
		ordered = ordered[:4]
	}
	return ordered
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}
