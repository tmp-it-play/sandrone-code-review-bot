package leaseheartbeat

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const renewalTimeout = 5 * time.Second

func Run(parent context.Context, leaseDuration time.Duration, renew func(context.Context) error, work func(context.Context) error) error {
	if leaseDuration <= 0 {
		return fmt.Errorf("lease 유지 시간이 올바르지 않습니다")
	}
	renewContext, renewCancel := context.WithTimeout(parent, renewalTimeout)
	err := renew(renewContext)
	renewCancel()
	if err != nil {
		return err
	}
	workContext, workCancel := context.WithCancelCause(parent)
	stop := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(leaseDuration / 4)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				heartbeatContext, heartbeatCancel := context.WithTimeout(workContext, renewalTimeout)
				heartbeatErr := renew(heartbeatContext)
				heartbeatCancel()
				if heartbeatErr != nil {
					workCancel(heartbeatErr)
					done <- heartbeatErr
					return
				}
			case <-stop:
				done <- nil
				return
			case <-workContext.Done():
				done <- nil
				return
			}
		}
	}()
	workErr := work(workContext)
	close(stop)
	heartbeatErr := <-done
	workCancel(context.Canceled)
	return errors.Join(workErr, heartbeatErr)
}
