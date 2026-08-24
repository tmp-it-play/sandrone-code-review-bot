package reviewanalysis

type EvidenceReport struct {
	Exact        int
	Normalized   int
	Reanchored   int
	UnknownFile  int
	InvalidSpan  int
	NonAddedSpan int
	NotFound     int
	Ambiguous    int
}

func (r EvidenceReport) Accepted() int {
	return r.Exact + r.Normalized + r.Reanchored
}

func (r EvidenceReport) Dropped() int {
	return r.UnknownFile + r.InvalidSpan + r.NonAddedSpan + r.NotFound + r.Ambiguous
}

func (r EvidenceReport) Add(other EvidenceReport) EvidenceReport {
	return EvidenceReport{
		Exact:        r.Exact + other.Exact,
		Normalized:   r.Normalized + other.Normalized,
		Reanchored:   r.Reanchored + other.Reanchored,
		UnknownFile:  r.UnknownFile + other.UnknownFile,
		InvalidSpan:  r.InvalidSpan + other.InvalidSpan,
		NonAddedSpan: r.NonAddedSpan + other.NonAddedSpan,
		NotFound:     r.NotFound + other.NotFound,
		Ambiguous:    r.Ambiguous + other.Ambiguous,
	}
}
