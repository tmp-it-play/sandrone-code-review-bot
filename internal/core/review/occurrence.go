package review

type Occurrence struct {
	ID       Fingerprint
	File     string
	Line     int
	EndLine  int
	Evidence string
}
