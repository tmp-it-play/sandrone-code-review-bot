package pullrequest

type Hunk struct {
	Start int
	End   int
}

func (h Hunk) Contains(line int) bool {
	return line >= h.Start && line <= h.End
}

func (h Hunk) Distance(line int) int {
	switch {
	case line < h.Start:
		return h.Start - line
	case line > h.End:
		return line - h.End
	default:
		return 0
	}
}
