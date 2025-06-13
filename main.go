package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/thushan/olla/pkg/profiler"
	"github.com/thushan/olla/theme"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/thushan/olla/internal/app"
	"github.com/thushan/olla/internal/env"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/internal/util"
	"github.com/thushan/olla/internal/version"
	"github.com/thushan/olla/pkg/format"
	"github.com/thushan/olla/pkg/nerdstats"
)

const (
	DefaultLoggerLevel   = "info"
	DefaultPrettyLogs    = true
	DefaultFileOutput    = true
	DefaultLogDir        = "./logs"
	DefaultLogSizeMB     = 1
	DefaultLogMaxBackups = 7
	DefaultLogMaxAgeDays = 14
	DefaultTheme         = "default"
)

var (
	enableProfiling bool
	showVersion     bool
)

func init() {
	flag.BoolVar(&enableProfiling, "profile", false, "Enable pprof profiling server")
	flag.BoolVar(&showVersion, "version", false, "Show version information")

	flag.Parse()

	if os.Getenv("OLLA_ENABLE_PROFILER") == "true" {
		enableProfiling = true
	}

	if os.Getenv("OLLA_SHOW_VERSION") == "true" {
		showVersion = true
	}
}
func main() {
	startTime := time.Now()
	vlog := log.New(log.Writer(), "", 0)
	profileAddress := "localhost:19841"

	if showVersion {
		version.PrintVersionInfo(true, vlog)
		os.Exit(0)
	} else {
		version.PrintVersionInfo(false, vlog)
	}

	if enableProfiling {
		profiler.InitialiseProfiler(profileAddress)
		vlog.Printf(theme.ColourProfiler("Profiling server started at http://%s/debug/pprof/\n"), profileAddress)
	}

	// Setup logging
	lcfg := buildLoggerConfig()
	logInstance, styledLogger, cleanup, err := logger.NewWithTheme(lcfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialise logger: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	slog.SetDefault(logInstance)

	styledLogger.Info("Initialising", "version", version.Version, "pid", os.Getpid())

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		styledLogger.Info("Shutdown signal received", "signal", sig.String())
		cancel()
	}()

	application, err := app.New(startTime, styledLogger)
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

	if application.Config.Engineering.ShowNerdStats {
		reportProcessStats(styledLogger, startTime)
	}

	styledLogger.Info("Olla has shutdown")
}

func reportProcessStats(logger logger.StyledLogger, startTime time.Time) {
	runtime.GC()
	stats := nerdstats.Snapshot(startTime)

	logger.Info("Process Memory Stats",
		"heap_alloc", format.Bytes(stats.HeapAlloc),
		"heap_sys", format.Bytes(stats.HeapSys),
		"heap_inuse", format.Bytes(stats.HeapInuse),
		"heap_released", format.Bytes(stats.HeapReleased),
		"stack_inuse", format.Bytes(stats.StackInuse),
		"total_alloc", format.Bytes(stats.TotalAlloc),
		"memory_pressure", stats.GetMemoryPressure(),
	)

	logger.Info("Process Allocation Stats",
		"total_mallocs", stats.Mallocs,
		"total_frees", stats.Frees,
		"net_objects", util.SafeInt64Diff(stats.Mallocs, stats.Frees),
	)

	if stats.NumGC > 0 {
		logger.Info("Garbage Collection Stats",
			"num_gc_cycles", stats.NumGC,
			"last_gc", stats.LastGC.Format(time.RFC3339),
			"total_gc_time", format.Duration(stats.TotalGCTime),
			"gc_cpu_fraction", fmt.Sprintf("%.4f%%", stats.GCCPUFraction*100),
		)
	}

	logger.Info("Goroutine Stats",
		"num_goroutines", stats.NumGoroutines,
		"goroutine_health", stats.GetGoroutineHealthStatus(),
		"num_cgo_calls", stats.NumCgoCall,
	)

	logger.Info("Runtime Stats",
		"uptime", format.Duration(stats.Uptime),
		"go_version", stats.GoVersion,
		"num_cpu", stats.NumCPU,
		"gomaxprocs", stats.GOMAXPROCS,
	)

	if buildInfo := stats.GetBuildInfoSummary(); len(buildInfo) > 0 {
		var buildArgs []any
		for key, value := range buildInfo {
			buildArgs = append(buildArgs, key, value)
		}
		logger.Info("Build Info", buildArgs...)
	}

	logger.Info("Process Health Summary",
		"memory_pressure", stats.GetMemoryPressure(),
		"goroutine_status", stats.GetGoroutineHealthStatus(),
		"uptime", format.Duration(stats.Uptime),
		"avg_gc_pause", nerdstats.CalculateAverageGCPause(stats),
	)
}

func buildLoggerConfig() *logger.Config {
	return &logger.Config{
		Level:      env.GetEnvOrDefault("OLLA_LOG_LEVEL", DefaultLoggerLevel),
		PrettyLogs: env.GetEnvBoolOrDefault("OLLA_PRETTY_LOGS", DefaultPrettyLogs),
		FileOutput: env.GetEnvBoolOrDefault("OLLA_FILE_OUTPUT", DefaultFileOutput),
		LogDir:     env.GetEnvOrDefault("OLLA_LOG_DIR", DefaultLogDir),
		MaxSize:    env.GetEnvIntOrDefault("OLLA_LOG_SIZE_MB", DefaultLogSizeMB),
		MaxBackups: env.GetEnvIntOrDefault("OLLA_LOG_MAX_BACKUPS", DefaultLogMaxBackups),
		MaxAge:     env.GetEnvIntOrDefault("OLLA_LOG_MAX_AGE_DAYS", DefaultLogMaxAgeDays),
		Theme:      env.GetEnvOrDefault("OLLA_THEME", DefaultTheme),
	}
}
