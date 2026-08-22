package review

type UnreviewedFile struct {
	Path      string
	Additions int
	Deletions int
	Reason    string
}
