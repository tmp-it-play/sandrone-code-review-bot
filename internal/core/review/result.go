package review

type Result struct {
	Summary  Summary
	Findings []Finding
}

func (r Result) Inline() []Finding {
	return r.byPlacement(PlacementInline)
}

func (r Result) Fallback() []Finding {
	return r.byPlacement(PlacementFallback)
}

func (r Result) byPlacement(placement Placement) []Finding {
	selected := make([]Finding, 0, len(r.Findings))
	for _, finding := range r.Findings {
		if finding.Placement == placement {
			selected = append(selected, finding)
		}
	}
	return selected
}
