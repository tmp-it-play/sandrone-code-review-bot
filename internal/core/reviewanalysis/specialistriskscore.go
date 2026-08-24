package reviewanalysis

import "github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"

func (c SpecialistRiskClassifier) Score(file pullrequest.ChangedFile) int {
	risks := c.Classify([]pullrequest.ChangedFile{file})
	score := 0
	for _, risk := range risks {
		switch risk {
		case "보안·권한 경계":
			score += 70
		case "데이터 무결성·마이그레이션":
			score += 60
		case "동시성·재시도·멱등성":
			score += 50
		case "API 계약·호환성":
			score += 40
		case "배포·공급망·롤백":
			score += 30
		case "불완전 diff·주변 문맥":
			score += 20
		case "삭제 영향·하위 호환성":
			score += 10
		}
	}
	return score
}
