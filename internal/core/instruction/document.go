package instruction

type Document struct {
	Path      string
	Content   string
	Truncated bool
}

func (d Document) Size() int {
	return len(d.Content)
}
