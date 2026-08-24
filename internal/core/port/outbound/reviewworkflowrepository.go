package outbound

type ReviewWorkflowRepository interface {
	ReviewRunRepository
	ReviewExecutionRepository
	ReviewVerificationRepository
	ReviewPublicationRepository
}
