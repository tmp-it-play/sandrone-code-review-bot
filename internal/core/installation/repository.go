package installation

type Repository struct {
	InstallationID int64
	Owner          string
	Name           string
	Private        bool
}

func (r Repository) FullName() string {
	return r.Owner + "/" + r.Name
}
