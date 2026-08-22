package dashboard

type PageData struct {
	Base         string
	Title        string
	Active       string
	Error        string
	Repositories []RepositoryView
	Reviews      []ReviewView
	Review       ReviewView
	Findings     []FindingView
	Providers    []ProviderView
	Commands     []CommandView
	Queue        QueueView
}
