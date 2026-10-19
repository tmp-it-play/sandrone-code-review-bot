package textrevision

import (
	"errors"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

var protectedText = regexp.MustCompile("(?s)```.*?```|`[^`]+`|https?://[^\\s<>]+|[A-Za-z_][A-Za-z0-9_]*(?:[./][A-Za-z_][A-Za-z0-9_]*)+|[A-Za-z_][A-Za-z0-9_]*\\([^()\\n]*\\)|[A-Za-z][A-Za-z0-9]*_[A-Za-z0-9_]+|[0-9]+(?:[.,][0-9]+)*")

func ValidateEdits(fields []Field, edits []Edit, complete bool) error {
	if complete && len(fields) != len(edits) {
		return errors.New("문체 보정 항목 수가 일치하지 않습니다")
	}
	known := make(map[string]Field, len(fields))
	for _, field := range fields {
		known[field.ID] = field
	}
	seen := make(map[string]bool, len(edits))
	for _, edit := range edits {
		field, exists := known[edit.ID]
		if !exists || seen[edit.ID] || strings.TrimSpace(edit.Text) == "" || !utf8.ValidString(edit.Text) || utf8.RuneCountInString(edit.Text) > field.Limit {
			return errors.New("문체 보정 항목이 누락, 중복되거나 형식이 잘못되었습니다")
		}
		seen[edit.ID] = true
		before := protectedText.FindAllString(field.Text, -1)
		after := protectedText.FindAllString(edit.Text, -1)
		slices.Sort(before)
		slices.Sort(after)
		if !slices.Equal(before, after) {
			return errors.New("문체 보정이 코드, 링크 또는 수치를 변경했습니다")
		}
		for _, marker := range []string{"<", ">", "```", "~~~"} {
			if strings.Count(edit.Text, marker) != strings.Count(field.Text, marker) {
				return errors.New("문체 보정이 마크다운 구조를 변경했습니다")
			}
		}
	}
	return nil
}
