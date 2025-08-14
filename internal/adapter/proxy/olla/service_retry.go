package olla

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/thushan/olla/internal/adapter/proxy/common"
	"github.com/thushan/olla/internal/adapter/proxy/core"
	"github.com/thushan/olla/internal/app/middleware"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/internal/util"
)

// ProxyRequestToEndpointsWithRetry proxies the request with retry logic for connection failures
func (s *Service) ProxyRequestToEndpointsWithRetry(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) error {
	s.IncrementRequests()

	// Use context logger if available
	ctxLogger := middleware.GetLogger(ctx)
	if ctxLogger != nil {
		ctxLogger.Debug("Olla proxy request started",
			"method", r.Method,
			"url", r.URL.String(),
			"endpoint_count", len(endpoints))
	} else {
		rlog.Debug("proxy request started", "method", r.Method, "url", r.URL.String())
	}

	if len(endpoints) == 0 {
		if ctxLogger != nil {
			ctxLogger.Error("No healthy endpoints available for request")
		} else {
			rlog.Error("no healthy endpoints available")
		}
		s.RecordFailure(ctx, nil, time.Since(stats.StartTime), common.ErrNoHealthyEndpoints)
		return common.ErrNoHealthyEndpoints
	}

	// Define the proxy function for a single endpoint
	proxyFunc := func(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoint *domain.Endpoint, stats *ports.RequestStats) error {
		return s.proxyToSingleEndpoint(ctx, w, r, endpoint, stats, rlog)
	}

	// Use the shared retry handler
	return s.retryHandler.ExecuteWithRetry(ctx, w, r, endpoints, s.Selector, stats, proxyFunc)
}

// proxyToSingleEndpoint handles proxying to a single endpoint with Olla's optimizations
func (s *Service) proxyToSingleEndpoint(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoint *domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) error {
	stats.EndpointName = endpoint.Name

	// Check circuit breaker first
	cb := s.GetCircuitBreaker(endpoint.Name)
	if cb != nil && cb.IsOpen() {
		rlog.Warn("Circuit breaker is open for endpoint", "endpoint", endpoint.Name)
		s.RecordFailure(ctx, endpoint, time.Since(stats.StartTime), fmt.Errorf("circuit breaker open"))
		return fmt.Errorf("circuit breaker open for endpoint %s", endpoint.Name)
	}

	ctxLogger := middleware.GetLogger(ctx)
	if ctxLogger != nil {
		ctxLogger.Info("Request dispatching",
			"endpoint", endpoint.Name,
			"target", stats.TargetUrl,
			"model", stats.Model)
	} else {
		rlog.Info("Request dispatching", "endpoint", endpoint.Name, "target", stats.TargetUrl, "model", stats.Model)
	}

	s.Selector.IncrementConnections(endpoint)
	defer s.Selector.DecrementConnections(endpoint)

	// Strip route prefix from request path for upstream
	targetPath := util.StripPrefix(r.URL.Path, s.configuration.GetProxyPrefix())
	targetURL := endpoint.URL.ResolveReference(&url.URL{Path: targetPath})
	if r.URL.RawQuery != "" {
		targetURL.RawQuery = r.URL.RawQuery
	}

	stats.TargetUrl = targetURL.String()

	// Get endpoint-specific connection pool and transport
	pool := s.getOrCreateEndpointPool(endpoint.Name)
	transport := pool.transport

	proxyReq, err := s.prepareProxyRequest(ctx, r, targetURL, stats)
	if err != nil {
		if cb != nil {
			cb.RecordFailure()
		}
		s.RecordFailure(ctx, endpoint, time.Since(stats.StartTime), err)
		return fmt.Errorf("failed to create proxy request: %w", err)
	}

	rlog.Debug("making round-trip request", "target", targetURL.String())
	backendStart := time.Now()
	resp, err := transport.RoundTrip(proxyReq)
	stats.BackendResponseMs = time.Since(backendStart).Milliseconds()

	if err != nil {
		if cb != nil {
			cb.RecordFailure()
		}
		// Don't log as error if it's a connection failure - the retry handler will handle it
		if core.IsConnectionError(err) {
			rlog.Debug("round-trip connection failed", "error", err)
		} else {
			rlog.Error("round-trip failed", "error", err)
		}
		s.RecordFailure(ctx, endpoint, time.Since(stats.StartTime), err)
		duration := time.Since(stats.StartTime)
		return common.MakeUserFriendlyError(err, duration, "backend", s.configuration.GetResponseTimeout())
	}
	defer resp.Body.Close()

	// Record success with circuit breaker
	if cb != nil {
		cb.RecordSuccess()
	}

	rlog.Debug("round-trip success", "status", resp.StatusCode)

	core.SetResponseHeaders(w, stats, endpoint)

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode)

	// Stream the response through with Olla's optimizations
	rlog.Debug("starting response stream")
	streamStart := time.Now()
	stats.FirstDataMs = time.Since(stats.StartTime).Milliseconds()

	buffer := s.bufferPool.Get()
	defer s.bufferPool.Put(buffer)

	// Stream with Olla's optimized streaming
	bytesWritten, streamErr := s.streamResponse(ctx, ctx, w, resp, *buffer, rlog)
	stats.StreamingMs = time.Since(streamStart).Milliseconds()
	stats.TotalBytes = bytesWritten

	if streamErr != nil && !errors.Is(streamErr, context.Canceled) {
		rlog.Error("streaming failed", "error", streamErr)
		s.RecordFailure(ctx, endpoint, time.Since(stats.StartTime), streamErr)
		return common.MakeUserFriendlyError(streamErr, time.Since(stats.StartTime), "streaming", s.configuration.GetResponseTimeout())
	}

	// We've successfully written the response
	duration := time.Since(stats.StartTime)
	s.RecordSuccess(endpoint, duration.Milliseconds(), int64(bytesWritten))

	s.PublishEvent(core.ProxyEvent{
		Type:      core.EventTypeProxySuccess,
		RequestID: stats.RequestID,
		Endpoint:  endpoint.Name,
		Duration:  duration,
		Metadata: core.ProxyEventMetadata{
			BytesSent:  int64(bytesWritten),
			StatusCode: resp.StatusCode,
			Model:      stats.Model,
		},
	})

	// Stats update
	stats.EndTime = time.Now()
	stats.Latency = stats.EndTime.Sub(stats.StartTime).Milliseconds()
	stats.TotalBytes = bytesWritten

	// Log detailed completion metrics at Debug level
	if ctxLogger != nil {
		ctxLogger.Debug("Olla proxy metrics",
			"endpoint", endpoint.Name,
			"latency_ms", stats.Latency,
			"processing_ms", stats.RequestProcessingMs,
			"backend_ms", stats.BackendResponseMs,
			"first_data_ms", stats.FirstDataMs,
			"streaming_ms", stats.StreamingMs,
			"selection_ms", stats.SelectionMs,
			"header_ms", stats.HeaderProcessingMs,
			"total_bytes", stats.TotalBytes,
			"bytes_formatted", middleware.FormatBytes(int64(stats.TotalBytes)),
			"status", resp.StatusCode,
			"request_id", middleware.GetRequestID(ctx))
	}

	return nil
}

// ProxyRequestWithRetry is an alias for ProxyRequestToEndpointsWithRetry
func (s *Service) ProxyRequestWithRetry(ctx context.Context, w http.ResponseWriter, r *http.Request, stats *ports.RequestStats, rlog logger.StyledLogger) error {
	endpoints, err := s.DiscoveryService.GetHealthyEndpoints(ctx)
	if err != nil {
		return err
	}
	return s.ProxyRequestToEndpointsWithRetry(ctx, w, r, endpoints, stats, rlog)
}
