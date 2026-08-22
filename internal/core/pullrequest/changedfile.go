package pullrequest

type ChangedFile struct {
	Path         string
	PreviousPath string
	Status       string
	Additions    int
	Deletions    int
	Patch        string
	Content      string
	Truncated    bool
}

func (f ChangedFile) IsRemoved() bool {
	return f.Status == "removed"
}

func (f ChangedFile) ChangeSize() int {
	return f.Additions + f.Deletions
}
