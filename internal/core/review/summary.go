package review

import "strings"

type Summary struct {
	Overview string
	Files    []FileNote
}

func (s Summary) IsEmpty() bool {
	return strings.TrimSpace(s.Overview) == "" && len(s.Files) == 0
}
