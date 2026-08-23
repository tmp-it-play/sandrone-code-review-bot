package diff

type Hunk struct {
	OldStart         int
	OldCount         int
	NewStart         int
	NewCount         int
	Section          string
	Body             string
	Hash             string
	DuplicateOrdinal int
	AddedLines       int
	DeletedLines     int
	Complete         bool
}
