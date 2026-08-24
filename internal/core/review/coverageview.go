package review

type CoverageView struct {
	Total    int
	Reviewed int
	Failed   int
	Deferred int
	Skipped  int
	Pending  int
	Status   string
}

func (v CoverageView) IsEmpty() bool {
	return v.Total == 0
}
