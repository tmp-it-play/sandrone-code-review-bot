package reviewpublication

type Dependencies struct {
	Publisher Publisher
	Receipts  ReceiptRepository
	Ownership ProgressMarkerOwnership
	Clock     Clock
	Logger    Logger
}
