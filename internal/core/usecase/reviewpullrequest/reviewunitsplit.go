package reviewpullrequest

import (
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

type reviewUnitSplit struct {
	children    []reviewworkflow.Unit
	assignments []reviewworkflow.CoverageAssignment
	works       []reviewUnitWork
}
