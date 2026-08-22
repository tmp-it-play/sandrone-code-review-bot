package dashboard

type QueueView struct {
	Pending   int
	Active    int
	Retry     int
	Archived  int
	Completed int
	Available bool
}
