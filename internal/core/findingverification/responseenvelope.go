package findingverification

type responseEnvelope struct {
	Decisions []responseEntry `json:"decisions"`
}
