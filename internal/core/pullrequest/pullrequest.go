package pullrequest

type PullRequest struct {
	Number  int
	Title   string
	Body    string
	Author  string
	BaseRef string
	HeadRef string
	BaseSHA string
	HeadSHA string
	State   string
	Draft   bool
}
