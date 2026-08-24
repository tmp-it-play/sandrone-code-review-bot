package webhookrecovery

type ScanResult struct {
	Deliveries []Delivery
	NextCursor string
}
