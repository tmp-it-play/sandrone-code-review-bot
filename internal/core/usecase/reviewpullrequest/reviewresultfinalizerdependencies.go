package reviewpullrequest

import "log/slog"

type reviewResultFinalizerDependencies struct {
	findings findingFingerprintRepository
	verifier *findingVerifier
	logger   *slog.Logger
}
