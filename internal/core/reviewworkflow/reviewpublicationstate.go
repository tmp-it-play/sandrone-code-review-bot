package reviewworkflow

const ReviewPublicationStatusPrepared = "prepared"
const ReviewPublicationStatusCompleted = "completed"

const ReviewPublicationChannelReview = "review"
const ReviewPublicationChannelComment = "comment"
const ReviewPublicationChannelReconciled = "reconciled"

func ValidReviewPublicationChannel(channel string, externalID int64) bool {
	switch channel {
	case ReviewPublicationChannelReview, ReviewPublicationChannelComment:
		return externalID > 0
	case ReviewPublicationChannelReconciled:
		return externalID >= 0
	default:
		return false
	}
}
