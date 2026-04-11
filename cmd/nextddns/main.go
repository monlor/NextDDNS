package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"nextddns/internal/app"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "/app/config.yaml", "path to YAML config")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}))

	application, err := app.New(configPath, logger)
	if err != nil {
		logger.Error("failed to initialize application", "error", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := application.Run(ctx); err != nil {
		logger.Error("application exited with error", "error", err)
		os.Exit(1)
	}
}
