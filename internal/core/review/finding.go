package review

import "strings"

type Finding struct {
	File       string
	Line       int
	EndLine    int
	Severity   Severity
	Title      string
	Body       string
	Suggestion string
	Placement  Placement
}

func (f Finding) IsValid() bool {
	return strings.TrimSpace(f.File) != "" && strings.TrimSpace(f.Body) != "" && f.Severity.Rank() > 0
}

func (f Finding) SpanStart() int {
	if f.EndLine > 0 && f.EndLine < f.Line {
		return f.EndLine
	}
	return f.Line
}

func (f Finding) SpanEnd() int {
	if f.EndLine > f.Line {
		return f.EndLine
	}
	return f.Line
}

func (f Finding) WithPlacement(placement Placement) Finding {
	f.Placement = placement
	return f
}

func (f Finding) WithLine(line int) Finding {
	f.Line = line
	f.EndLine = 0
	return f
}
