package pullrequest

import "strconv"

type Target struct {
	InstallationID int64
	Owner          string
	Repository     string
	Number         int
	HeadSHA        string
	BaseSHA        string
}

func (t Target) FullName() string {
	return t.Owner + "/" + t.Repository
}

func (t Target) Reference() string {
	return t.FullName() + "#" + strconv.Itoa(t.Number)
}

func (t Target) IsComplete() bool {
	return t.Owner != "" && t.Repository != "" && t.Number > 0
}
