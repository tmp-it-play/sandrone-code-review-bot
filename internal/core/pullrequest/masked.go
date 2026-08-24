package pullrequest

func (p PullRequest) Masked(mask func(string) string) PullRequest {
	p.Title = mask(p.Title)
	p.Body = mask(p.Body)
	p.Author = mask(p.Author)
	p.BaseRef = mask(p.BaseRef)
	p.HeadRef = mask(p.HeadRef)
	return p
}
