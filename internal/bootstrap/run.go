package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

func (a *Application) Run(ctx context.Context) error {
	if err := a.asynqServer.Start(a.asynqMux); err != nil {
		a.closeResources()
		return fmt.Errorf("워커를 시작하지 못했습니다: %w", err)
	}
	errs := make(chan error, 1)
	go func() {
		a.logger.Info("HTTP 서버를 시작합니다", "address", a.httpServer.Addr)
		if err := a.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- err
			return
		}
		errs <- nil
	}()
	maintenanceContext, stopMaintenance := context.WithCancel(ctx)
	var maintenanceWorkers sync.WaitGroup
	for _, run := range []func(context.Context){
		a.reviewRetention.Run,
		a.reviewPublication.Run,
		a.progressComment.Run,
		a.webhookInbox.Run,
		a.webhookRecovery.Run,
		a.occurrenceBootstrap.Run,
	} {
		maintenanceWorkers.Add(1)
		go func(run func(context.Context)) {
			defer maintenanceWorkers.Done()
			run(maintenanceContext)
		}(run)
	}

	select {
	case <-ctx.Done():
	case err := <-errs:
		stopMaintenance()
		maintenanceWorkers.Wait()
		a.shutdown()
		return err
	}
	stopMaintenance()
	maintenanceWorkers.Wait()
	a.shutdown()
	return nil
}

func (a *Application) shutdown() {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := a.httpServer.Shutdown(shutdownCtx); err != nil {
		a.logger.Warn("HTTP 서버를 정상 종료하지 못했습니다", "error", err)
	}
	a.asynqServer.Shutdown()
	a.closeResources()
	a.logger.Info("종료했습니다")
}
