package main

import (
	"context"
	"fmt"
	"github.com/thushan/olla/app"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/version"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
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

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// setup: logging
	lcfg := buildLoggerConfig()
	log, cleanup, err := logger.New(lcfg)
	if err != nil {
		fmt.Printf("Failed to initialise logger: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	// Set as default logger
	slog.SetDefault(log)

	log.Info("Initialising", "version", version.Version, "pid", os.Getpid())
	
	// setup: graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		log.Info("Shutdown signal received", "signal", sig.String())
		cancel()
	}()

	application, err := app.New(cfg, log)
	if err != nil {
		log.Error("Failed to create application: %v", err)
	}

	if err := application.Start(ctx); err != nil {
		log.Error("Failed to start application: %v", err)
	}

	<-ctx.Done()

	if err := application.Stop(context.Background()); err != nil {
		log.Error("Error during shutdown: %v", err)
	}

	log.Info("Olla has shutdown")
}

// buildLoggerConfig creates logger config from environment variables with defaults
func buildLoggerConfig() *logger.Config {
	return &logger.Config{
		Level:      getEnvOrDefault("OLLA_LOG_LEVEL", "info"),
		FileOutput: getEnvBoolOrDefault("OLLA_FILE_OUTPUT", true),
		LogDir:     getEnvOrDefault("OLLA_LOG_DIR", "./logs"),
		MaxSize:    getEnvIntOrDefault("OLLA_MAX_SIZE", 100),
		MaxBackups: getEnvIntOrDefault("OLLA_MAX_BACKUPS", 5),
		MaxAge:     getEnvIntOrDefault("OLLA_MAX_AGE", 30),
		Theme:      getEnvOrDefault("OLLA_THEME", "default"),
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvBoolOrDefault(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func getEnvIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}
