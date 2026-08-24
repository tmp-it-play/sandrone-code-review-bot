package review

import "strings"

type Finding struct {
	File         string
	Line         int
	EndLine      int
	Severity     Severity
	Title        string
	Body         string
	Suggestion   string
	Evidence     string
	RootCause    string
	RootCauseID  Fingerprint
	OccurrenceID Fingerprint
	Occurrences  []Occurrence
	Placement    Placement
	Snapped      bool
}

func (f Finding) IsValid() bool {
	return strings.TrimSpace(f.File) != "" &&
		f.Line > 0 &&
		(f.EndLine == 0 || f.EndLine >= f.Line) &&
		strings.TrimSpace(f.Title) != "" &&
		strings.TrimSpace(f.Body) != "" &&
		strings.TrimSpace(f.Evidence) != "" &&
		strings.TrimSpace(f.RootCause) != "" &&
		f.Severity.Rank() > 0
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
	f.Snapped = f.Line != line
	f.Line = line
	f.EndLine = 0
	return f
}

func (f Finding) WithIdentity() Finding {
	f.RootCauseID = NewRootCauseFingerprint(f)
	f.OccurrenceID = NewOccurrenceFingerprint(f)
	endLine := f.EndLine
	if endLine == f.Line {
		endLine = 0
	}
	primary := Occurrence{
		ID:       f.OccurrenceID,
		File:     f.File,
		Line:     f.Line,
		EndLine:  endLine,
		Evidence: f.Evidence,
	}
	found := false
	for _, occurrence := range f.Occurrences {
		if occurrence.ID == primary.ID {
			found = true
			break
		}
	}
	if !found {
		f.Occurrences = append(f.Occurrences, primary)
	}
	return f
}

func (f Finding) SuggestionApplies() bool {
	return f.Placement == PlacementInline && !f.Snapped && (f.EndLine <= 0 || f.EndLine == f.Line) && strings.TrimSpace(f.Suggestion) != ""
}
