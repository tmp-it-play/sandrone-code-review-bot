package findingverification

type Status string

const (
	StatusSupported   Status = "supported"
	StatusUnsupported Status = "unsupported"
	StatusUncertain   Status = "uncertain"
)

func (s Status) Valid() bool {
	switch s {
	case StatusSupported, StatusUnsupported, StatusUncertain:
		return true
	default:
		return false
	}
}
