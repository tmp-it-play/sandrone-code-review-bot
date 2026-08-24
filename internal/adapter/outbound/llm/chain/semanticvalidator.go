package chain

import (
	"fmt"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/parsing"
)

func validateSemanticResponse(validation llm.ResponseValidation, content string) error {
	var err error
	switch validation.Policy {
	case "":
		return nil
	case llm.ResponseValidationReviewResult:
		err = (parsing.ReviewResponseValidator{RequiredPaths: validation.RequiredPaths}).Validate(content)
	case llm.ResponseValidationSummaryResult:
		err = (parsing.SummaryResponseValidator{}).Validate(content)
	default:
		err = fmt.Errorf("알 수 없는 결과 검증 정책: %s", validation.Policy)
	}
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSemanticResponse, err)
	}
	return nil
}
