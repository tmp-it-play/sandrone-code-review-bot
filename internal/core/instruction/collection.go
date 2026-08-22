package instruction

type Collection struct {
	Documents []Document
	Omitted   []string
}

func (c Collection) IsEmpty() bool {
	return len(c.Documents) == 0
}

func (c Collection) TotalSize() int {
	total := 0
	for _, document := range c.Documents {
		total += document.Size()
	}
	return total
}
