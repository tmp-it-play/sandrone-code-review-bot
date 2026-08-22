package review

import "strings"

type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityMajor    Severity = "major"
	SeverityMinor    Severity = "minor"
	SeverityNit      Severity = "nit"
)

var severityRanks = map[Severity]int{
	SeverityNit:      1,
	SeverityMinor:    2,
	SeverityMajor:    3,
	SeverityCritical: 4,
}

var severityLabels = map[Severity]string{
	SeverityNit:      "사소",
	SeverityMinor:    "경미",
	SeverityMajor:    "중요",
	SeverityCritical: "치명",
}

func ParseSeverity(value string) (Severity, bool) {
	candidate := Severity(strings.ToLower(strings.TrimSpace(value)))
	if _, ok := severityRanks[candidate]; !ok {
		return "", false
	}
	return candidate, true
}

func (s Severity) Rank() int {
	return severityRanks[s]
}

func (s Severity) AtLeast(minimum Severity) bool {
	return s.Rank() >= minimum.Rank()
}

func (s Severity) Label() string {
	label, ok := severityLabels[s]
	if !ok {
		return string(s)
	}
	return label
}
