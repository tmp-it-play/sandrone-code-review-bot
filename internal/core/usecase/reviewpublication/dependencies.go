package reviewpublication

type Dependencies struct {
	Publisher Publisher
	Receipts  ReceiptRepository
	Ownership ProgressMarkerOwnership
	Checks    ProgressCheck
	Clock     Clock
	Logger    Logger
}
