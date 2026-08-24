package chain

type ProviderMetrics interface {
	ObserveProvider(provider string, outcome string)
}
