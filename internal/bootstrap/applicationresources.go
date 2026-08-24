package bootstrap

func (a *Application) closeResources() {
	a.resourceCloseOnce.Do(func() {
		if err := a.queueClient.Close(); err != nil {
			a.logger.Warn("큐 클라이언트를 닫지 못했습니다", "error", err)
		}
		if err := a.inspector.Close(); err != nil {
			a.logger.Warn("큐 인스펙터를 닫지 못했습니다", "error", err)
		}
		if err := a.cache.Close(); err != nil {
			a.logger.Warn("Redis 연결을 닫지 못했습니다", "error", err)
		}
		if err := a.database.Close(); err != nil {
			a.logger.Warn("MySQL 연결을 닫지 못했습니다", "error", err)
		}
	})
}
