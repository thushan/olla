package app

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/thushan/olla/internal/util"
	"github.com/thushan/olla/pkg/format"
	"github.com/thushan/olla/pkg/nerdstats"
)

type ProcessStatsResponse struct {
	Timestamp time.Time `json:"timestamp"`
	Memory    struct {
		HeapAlloc      string `json:"heap_alloc"`
		HeapSys        string `json:"heap_sys"`
		HeapInuse      string `json:"heap_inuse"`
		HeapReleased   string `json:"heap_released"`
		StackInuse     string `json:"stack_inuse"`
		TotalAlloc     string `json:"total_alloc"`
		MemoryPressure string `json:"memory_pressure"`
	} `json:"memory"`

	GarbageCollection struct {
		LastGC        string  `json:"last_gc"`
		TotalGCTime   string  `json:"total_gc_time"`
		AvgGCPause    string  `json:"avg_gc_pause"`
		GCCPUFraction float64 `json:"gc_cpu_fraction"`
		NumGC         uint32  `json:"num_gc_cycles"`
	} `json:"garbage_collection"`

	Goroutines struct {
		HealthStatus string `json:"health_status"`
		Count        int    `json:"count"`
		CgoCalls     int64  `json:"cgo_calls"`
	} `json:"goroutines"`

	Runtime struct {
		Uptime     string `json:"uptime"`
		GoVersion  string `json:"go_version"`
		NumCPU     int    `json:"num_cpu"`
		GOMAXPROCS int    `json:"gomaxprocs"`
	} `json:"runtime"`

	Allocations struct {
		TotalMallocs uint64 `json:"total_mallocs"`
		TotalFrees   uint64 `json:"total_frees"`
		NetObjects   int64  `json:"net_objects"`
	} `json:"allocations"`
}

func (a *Application) processStatsHandler(w http.ResponseWriter, r *http.Request) {
	stats := nerdstats.Snapshot(a.StartTime)

	response := ProcessStatsResponse{
		Timestamp: time.Now(),
	}

	response.Memory.HeapAlloc = format.Bytes(stats.HeapAlloc)
	response.Memory.HeapSys = format.Bytes(stats.HeapSys)
	response.Memory.HeapInuse = format.Bytes(stats.HeapInuse)
	response.Memory.HeapReleased = format.Bytes(stats.HeapReleased)
	response.Memory.StackInuse = format.Bytes(stats.StackInuse)
	response.Memory.TotalAlloc = format.Bytes(stats.TotalAlloc)
	response.Memory.MemoryPressure = stats.GetMemoryPressure()

	response.Allocations.TotalMallocs = stats.Mallocs
	response.Allocations.TotalFrees = stats.Frees
	response.Allocations.NetObjects = util.SafeInt64Diff(stats.Mallocs, stats.Frees)

	response.GarbageCollection.NumGC = stats.NumGC
	if !stats.LastGC.IsZero() {
		response.GarbageCollection.LastGC = stats.LastGC.Format(time.RFC3339)
		response.GarbageCollection.TotalGCTime = format.Duration(stats.TotalGCTime)
		if stats.NumGC > 0 {
			avgPause := stats.TotalGCTime / time.Duration(stats.NumGC)
			response.GarbageCollection.AvgGCPause = format.Duration(avgPause)
		}
	}
	response.GarbageCollection.GCCPUFraction = stats.GCCPUFraction

	response.Goroutines.Count = stats.NumGoroutines
	response.Goroutines.HealthStatus = stats.GetGoroutineHealthStatus()
	response.Goroutines.CgoCalls = stats.NumCgoCall

	response.Runtime.Uptime = format.Duration(stats.Uptime)
	response.Runtime.GoVersion = stats.GoVersion
	response.Runtime.NumCPU = stats.NumCPU
	response.Runtime.GOMAXPROCS = stats.GOMAXPROCS

	w.Header().Set(ContentTypeHeader, ContentTypeJSON)
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		a.logger.Error("Failed to encode process stats response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
