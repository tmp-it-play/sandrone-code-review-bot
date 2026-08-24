package findingverification

type responseEntry struct {
	OccurrenceID string `json:"occurrenceId"`
	Status       string `json:"status"`
	Reason       string `json:"reason"`
}
