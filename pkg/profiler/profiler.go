package profiler

import (
	"log"
	"net/http"
	"net/http/pprof"
	"time"
)

// InitialiseProfiler sets up the HTTP server for pprof profiling.
// Based off  https://github.com/thushan/smash/blob/main/pkg/profiler/profiler.go
func InitialiseProfiler(address string) {
	http.DefaultServeMux = http.NewServeMux()
	go func() {
		server := &http.Server{
			Addr:         address,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		}

		http.HandleFunc("/debug/pprof/", pprof.Index)
		http.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		http.HandleFunc("/debug/pprof/profile", pprof.Profile)
		http.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		http.HandleFunc("/debug/pprof/trace", pprof.Trace)

		log.Println(server.ListenAndServe())
	}()
}
