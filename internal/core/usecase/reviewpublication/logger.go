package reviewpublication

type Logger interface {
	Warn(message string, args ...any)
}
