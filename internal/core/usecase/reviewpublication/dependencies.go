package reviewpublication

type Dependencies struct {
	Publisher Publisher
	Receipts  ReceiptRepository
	Clock     Clock
	Logger    Logger
}
