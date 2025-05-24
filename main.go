// main.go - Updated sections to use styled logger
package main

import (
	"context"
	"fmt"
	"github.com/thushan/olla/internal/app"
	"github.com/thushan/olla/internal/env"
	"github.com/thushan/olla/internal/version"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/thushan/olla/internal/logger"
)

func main() {
	vlog := log.New(log.Writer(), "", 0)
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		version.PrintVersionInfo(true, vlog)
		os.Exit(0)
	} else {
		version.PrintVersionInfo(false, vlog)
	}

	// setup: logging with styled logger
	lcfg := buildLoggerConfig()
	logInstance, styledLogger, cleanup, err := logger.NewWithTheme(lcfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialise logger: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	// Set as default logger
	slog.SetDefault(logInstance)

	styledLogger.Info("Initialising", "version", version.Version, "pid", os.Getpid())

	// setup: graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		styledLogger.Info("Shutdown signal received", "signal", sig.String())
		cancel()
	}()

	// Pass styled logger to application
	application, err := app.New(styledLogger)
	if err != nil {
		logger.FatalWithLogger(logInstance, "Failed to create application", "error", err)
	}

	if err := application.Start(ctx); err != nil {
		logger.FatalWithLogger(logInstance, "Failed to start application", "error", err)
	}

	<-ctx.Done()

	if err := application.Stop(context.Background()); err != nil {
		styledLogger.Error("Error during shutdown", "error", err)
	}

	styledLogger.Info("Olla has shutdown")
}

// buildLoggerConfig creates logger config from environment variables with defaults
func buildLoggerConfig() *logger.Config {
	return &logger.Config{
		Level:      env.GetEnvOrDefault("OLLA_LOG_LEVEL", "info"),
		FileOutput: env.GetEnvBoolOrDefault("OLLA_FILE_OUTPUT", true),
		LogDir:     env.GetEnvOrDefault("OLLA_LOG_DIR", "./logs"),
		MaxSize:    env.GetEnvIntOrDefault("OLLA_MAX_SIZE", 100),
		MaxBackups: env.GetEnvIntOrDefault("OLLA_MAX_BACKUPS", 5),
		MaxAge:     env.GetEnvIntOrDefault("OLLA_MAX_AGE", 30),
		Theme:      env.GetEnvOrDefault("OLLA_THEME", "default"),
	}
}
