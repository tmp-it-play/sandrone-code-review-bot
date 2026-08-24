package pullrequest

import "time"

type PullRequest struct {
	Number       int
	Title        string
	Body         string
	Author       string
	BaseRef      string
	HeadRef      string
	BaseSHA      string
	HeadSHA      string
	State        string
	Draft        bool
	Private      bool
	ChangedFiles int
	UpdatedAt    time.Time
}
