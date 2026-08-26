package reviewpublication

import "context"

type Reconciler struct {
	deps         ReconcilerDependencies
	config       ReconcilerConfig
	afterID      uint64
	throughID    uint64
	publications *PreparedPublisher
}

func NewReconciler(deps ReconcilerDependencies, config ReconcilerConfig) *Reconciler {
	return &Reconciler{
		deps:   deps,
		config: config,
		publications: New(Dependencies{
			Publisher: deps.Publisher,
			Receipts:  deps.Publications,
			Ownership: deps.Runs,
			Clock:     deps.Clock,
			Logger:    deps.Logger,
		}),
	}
}

func (r *Reconciler) Reconcile(ctx context.Context) {
	r.reconcileInvalidations(ctx)
	r.reconcileRuns(ctx)
}
