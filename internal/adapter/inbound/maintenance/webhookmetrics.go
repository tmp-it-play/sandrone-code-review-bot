package maintenance

type WebhookMetrics interface {
	ObserveWebhook(event string, action string)
}
