package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/it-play/sandrone-code-review-bot/internal/bootstrap"
)

func main() {
	config, err := bootstrap.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "설정을 읽지 못했다:", err)
		os.Exit(1)
	}
	application, err := bootstrap.NewApplication(config)
	if err != nil {
		fmt.Fprintln(os.Stderr, "애플리케이션을 준비하지 못했다:", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := application.Run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "실행 중 오류가 발생했다:", err)
		os.Exit(1)
	}
}
