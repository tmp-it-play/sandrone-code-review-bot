package pullrequest

import "strconv"

type Target struct {
	InstallationID int64
	Owner          string
	Repository     string
	Number         int
	HeadSHA        string
	BaseSHA        string
	BaseRef        string
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

func (t Target) ContentRefs() []string {
	refs := make([]string, 0, 3)
	for _, candidate := range []string{t.HeadSHA, t.BaseRef} {
		if candidate == "" {
			continue
		}
		if !containsRef(refs, candidate) {
			refs = append(refs, candidate)
		}
	}
	return append(refs, "")
}

func (t Target) PolicyRefs() []string {
	if t.HeadSHA != "" {
		return []string{t.HeadSHA}
	}
	return []string{""}
}

func containsRef(refs []string, candidate string) bool {
	for _, ref := range refs {
		if ref == candidate {
			return true
		}
	}
	return false
}
