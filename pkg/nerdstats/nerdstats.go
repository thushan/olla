package nerdstats

import (
	"github.com/thushan/olla/pkg/format"
	"runtime"
	"runtime/debug"
	"time"
)

/*
	NerdStats v2 provides a comprehensive snapshot of Go runtime statistics.
	It includes memory usage, garbage collection stats, goroutine counts etc.

	See: https://pkg.go.dev/runtime#MemStats for more details on the fields.

	Usage:
		stats := nerdstats.Snapshot(time.Now())
		fmt.Println(stats)

	v1: https://github.com/thushan/smash/blob/main/pkg/nerdstats/nerdstats.go
*/

type NerdStats struct {
	// Memory stats
	HeapAlloc    uint64 // Allocated heap memory in bytes
	HeapSys      uint64 // Heap memory obtained from OS
	HeapInuse    uint64 // Heap memory in use
	HeapReleased uint64 // Heap memory released to OS
	StackInuse   uint64 // Stack memory in use
	StackSys     uint64 // Stack memory obtained from OS
	TotalAlloc   uint64 // Total bytes allocated (cumulative)
	Mallocs      uint64 // Number of malloc operations
	Frees        uint64 // Number of free operations

	// Garbage collection stats
	NumGC         uint32        // Number of GC cycles
	LastGC        time.Time     // Time of last GC
	TotalGCTime   time.Duration // Total time spent in GC
	GCCPUFraction float64       // Fraction of CPU time used by GC

	// Goroutine stats
	NumGoroutines int   // Number of goroutines
	NumCgoCall    int64 // Number of cgo calls

	// Runtime stats
	NumCPU     int           // Number of logical CPUs
	GOMAXPROCS int           // Max OS threads for Go
	GoVersion  string        // Go version
	Uptime     time.Duration // Process uptime

	// Debug info
	BuildInfo *debug.BuildInfo // Build information if available
}

func Snapshot(startTime time.Time) *NerdStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	stats := &NerdStats{
		HeapAlloc:    m.HeapAlloc,
		HeapSys:      m.HeapSys,
		HeapInuse:    m.HeapInuse,
		HeapReleased: m.HeapReleased,
		StackInuse:   m.StackInuse,
		StackSys:     m.StackSys,
		TotalAlloc:   m.TotalAlloc,
		Mallocs:      m.Mallocs,
		Frees:        m.Frees,

		// GC stats
		NumGC:         m.NumGC,
		GCCPUFraction: m.GCCPUFraction,

		// Goroutine stats
		NumGoroutines: runtime.NumGoroutine(),
		NumCgoCall:    runtime.NumCgoCall(),

		// Runtime stats
		NumCPU:     runtime.NumCPU(),
		GOMAXPROCS: runtime.GOMAXPROCS(0),
		GoVersion:  runtime.Version(),
		Uptime:     time.Since(startTime),
	}

	// Calculate last GC time and total GC time
	if m.LastGC > 0 {
		stats.LastGC = time.Unix(0, int64(m.LastGC))
		stats.TotalGCTime = time.Duration(m.PauseTotalNs)
	}

	// Get build info if available
	if info, ok := debug.ReadBuildInfo(); ok {
		stats.BuildInfo = info
	}

	return stats
}

// GetMemoryPressure returns a simple assessment of memory pressure
func (ps *NerdStats) GetMemoryPressure() string {
	heapUsageRatio := float64(ps.HeapInuse) / float64(ps.HeapSys)
	allocsPerFree := float64(ps.Mallocs) / float64(ps.Frees+1) // +1 to avoid division by zero

	if heapUsageRatio > 0.9 && allocsPerFree > 1.5 {
		return "HIGH"
	} else if heapUsageRatio > 0.7 || allocsPerFree > 1.2 {
		return "MEDIUM"
	}
	return "LOW"
}

// GetGoroutineHealthStatus assesses goroutine count health
func (ps *NerdStats) GetGoroutineHealthStatus() string {
	// These thresholds are conservative for Olla
	if ps.NumGoroutines > 1000 {
		return "CONCERNING"
	} else if ps.NumGoroutines > 500 {
		return "ELEVATED"
	} else if ps.NumGoroutines > 100 {
		return "NORMAL"
	}
	return "HEALTHY"
}

// GetBuildInfoSummary returns a summary of build information
func (ps *NerdStats) GetBuildInfoSummary() map[string]string {
	summary := make(map[string]string)

	if ps.BuildInfo == nil {
		return summary
	}

	summary["path"] = ps.BuildInfo.Path
	summary["main_version"] = ps.BuildInfo.Main.Version

	for _, setting := range ps.BuildInfo.Settings {
		switch setting.Key {
		case "CGO_ENABLED", "GOARCH", "GOOS", "vcs.revision", "vcs.time":
			summary[setting.Key] = setting.Value
		}
	}

	return summary
}

func CalculateAverageGCPause(stats *NerdStats) string {
	if stats.NumGC == 0 {
		return "N/A"
	}
	avgPause := stats.TotalGCTime / time.Duration(stats.NumGC)
	return format.Duration(avgPause)
}
