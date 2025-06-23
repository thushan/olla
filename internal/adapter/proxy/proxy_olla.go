package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/internal/util"
	"github.com/thushan/olla/internal/version"
)

type OllaProxyService struct {
	discoveryService ports.DiscoveryService
	selector         domain.EndpointSelector
	transport        *http.Transport
	configuration    *OllaConfiguration
	statsCollector   ports.StatsCollector
	logger           logger.StyledLogger

	stats struct {
		totalRequests      int64
		successfulRequests int64
		failedRequests     int64
		totalLatency       int64
	}
}

func NewOllaService(
	discoveryService ports.DiscoveryService,
	selector domain.EndpointSelector,
	configuration *OllaConfiguration,
	statsCollector ports.StatsCollector,
	logger logger.StyledLogger,
) *OllaProxyService {
	if configuration.ConnectionTimeout == 0 {
		configuration.ConnectionTimeout = 30 * time.Second
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialer := &net.Dialer{
				Timeout: configuration.ConnectionTimeout,
			}
			return dialer.DialContext(ctx, network, addr)
		},
	}

	return &OllaProxyService{
		discoveryService: discoveryService,
		selector:         selector,
		transport:        transport,
		configuration:    configuration,
		statsCollector:   statsCollector,
		logger:           logger,
	}
}

// ProxyRequest handles incoming HTTP requests and proxies them to healthy endpoints
func (s *OllaProxyService) ProxyRequest(ctx context.Context, w http.ResponseWriter, r *http.Request, stats *ports.RequestStats, rlog logger.StyledLogger) error {
	endpoints, err := s.discoveryService.GetHealthyEndpoints(ctx)
	if err != nil {
		return err
	}

	return s.ProxyRequestToEndpoints(ctx, w, r, endpoints, stats, rlog)
}

// ProxyRequestToEndpoints proxies the request to the provided endpoints
func (s *OllaProxyService) ProxyRequestToEndpoints(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) error {
	startTime := time.Now()

	handleError := func(endpoint *domain.Endpoint, err error, statusCode int, bytes int64) error {
		duration := time.Since(startTime)
		s.updateStats(false, duration)
		s.recordFailure(endpoint, duration, bytes)

		rlog.Error("Proxy request failed",
			"error", err,
			"duration", duration,
			"endpoint", func() string {
				if endpoint != nil {
					return endpoint.Name
				}
				return "none"
			}())

		return domain.NewProxyError(
			stats.RequestID,
			stats.TargetUrl,
			r.Method,
			r.URL.Path,
			statusCode,
			duration,
			int(bytes),
			err,
		)
	}

	rlog.Debug("Proxy request started", "method", r.Method, "url", r.URL.String())

	if len(endpoints) == 0 {
		return handleError(nil, fmt.Errorf("no healthy AI backends available"), 0, 0)
	}

	selectionStart := time.Now()
	endpoint, err := s.selector.Select(ctx, endpoints)
	if err != nil {
		return handleError(nil, fmt.Errorf("failed to select endpoint: %w", err), 0, 0)
	}
	stats.SelectionMs = time.Since(selectionStart).Milliseconds()

	targetPath := s.stripRoutePrefix(r.Context(), r.URL.Path)
	targetURL := endpoint.URL.ResolveReference(&url.URL{Path: targetPath})
	if r.URL.RawQuery != "" {
		targetURL.RawQuery = r.URL.RawQuery
	}

	stats.EndpointName = endpoint.Name
	stats.TargetUrl = targetURL.String()

	rlog.Info("Proxying request", "endpoint", endpoint.Name, "target", targetURL.String())

	proxyCtx := ctx
	if s.configuration.ResponseTimeout > 0 {
		var cancel context.CancelFunc
		proxyCtx, cancel = context.WithTimeout(ctx, s.configuration.ResponseTimeout)
		defer cancel()
	}

	requestStart := time.Now()
	proxyReq, err := http.NewRequestWithContext(proxyCtx, r.Method, targetURL.String(), r.Body)
	if err != nil {
		return handleError(endpoint, fmt.Errorf("failed to create proxy request: %w", err), 0, 0)
	}

	headerStart := time.Now()
	s.copyHeaders(proxyReq, r)
	stats.HeaderProcessingMs = time.Since(headerStart).Milliseconds()
	stats.RequestProcessingMs = time.Since(requestStart).Milliseconds()

	backendStart := time.Now()
	resp, err := s.transport.RoundTrip(proxyReq)
	if err != nil {
		return handleError(endpoint, s.makeUserFriendlyError(err), 0, 0)
	}
	defer resp.Body.Close()
	stats.BackendResponseMs = time.Since(backendStart).Milliseconds()

	rlog.Debug("Backend responded", "status", resp.StatusCode, "backend_ms", stats.BackendResponseMs)

	s.selector.IncrementConnections(endpoint)
	defer s.selector.DecrementConnections(endpoint)
	for k, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	streamStart := time.Now()
	bytesWritten, err := s.streamResponse(w, resp.Body, rlog)
	if err != nil {
		return handleError(endpoint, fmt.Errorf("streaming failed: %w", err), resp.StatusCode, int64(bytesWritten))
	}
	stats.StreamingMs = time.Since(streamStart).Milliseconds()

	duration := time.Since(startTime)
	stats.TotalBytes = bytesWritten
	stats.EndTime = time.Now()
	stats.Latency = duration.Milliseconds()
	stats.FirstDataMs = streamStart.Sub(startTime).Milliseconds()

	s.updateStats(true, duration)
	s.statsCollector.RecordRequest(endpoint, "success", duration, int64(bytesWritten))

	rlog.Info("Request completed successfully",
		"latency_ms", stats.Latency,
		"backend_ms", stats.BackendResponseMs,
		"streaming_ms", stats.StreamingMs,
		"bytes", bytesWritten)

	return nil
}

// streamResponse handles response streaming with basic error detection
func (s *OllaProxyService) streamResponse(w http.ResponseWriter, body io.Reader, rlog logger.StyledLogger) (int, error) {
	buffer := make([]byte, 32*1024) // 32KB buffer
	totalBytes := 0

	flusher, canFlush := w.(http.Flusher)

	for {
		n, err := body.Read(buffer)
		if n > 0 {
			written, writeErr := w.Write(buffer[:n])
			if writeErr != nil {
				return totalBytes, fmt.Errorf("failed to write response: %w", writeErr)
			}
			totalBytes += written

			if canFlush {
				flusher.Flush()
			}

			rlog.Debug("Streamed chunk", "bytes", n, "total", totalBytes)
		}

		if err != nil {
			if err == io.EOF {
				rlog.Debug("Stream completed", "total_bytes", totalBytes)
				return totalBytes, nil
			}
			return totalBytes, fmt.Errorf("stream read error: %w", err)
		}
	}
}

func (s *OllaProxyService) stripRoutePrefix(ctx context.Context, path string) string {
	return util.StripRoutePrefix(ctx, path, s.configuration.ProxyPrefix)
}

func (s *OllaProxyService) copyHeaders(proxyReq, originalReq *http.Request) {
	for k, vals := range originalReq.Header {
		if len(vals) == 1 {
			proxyReq.Header.Set(k, vals[0])
		} else {
			proxyReq.Header[k] = make([]string, len(vals))
			copy(proxyReq.Header[k], vals)
		}
	}

	proxyReq.Header.Set("X-Forwarded-Host", originalReq.Host)
	if originalReq.TLS != nil {
		proxyReq.Header.Set("X-Forwarded-Proto", "https")
	} else {
		proxyReq.Header.Set("X-Forwarded-Proto", "http")
	}

	if ip, _, err := net.SplitHostPort(originalReq.RemoteAddr); err == nil {
		proxyReq.Header.Set("X-Forwarded-For", ip)
	}

	proxyReq.Header.Set("X-Proxied-By", fmt.Sprintf("%s/%s", version.Name, version.Version))
	proxyReq.Header.Set("Via", fmt.Sprintf("1.1 %s/%s", version.ShortName, version.Version))
}

// makeUserFriendlyError converts technical errors to user-friendly messages
func (s *OllaProxyService) makeUserFriendlyError(err error) error {
	if err == nil {
		return nil
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return fmt.Errorf("AI backend took too long to respond (timeout)")
		}
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Op == "dial" {
			return fmt.Errorf("could not connect to AI backend (connection refused)")
		}
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("AI backend response timeout exceeded")
	}
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("request was cancelled")
	}

	return err
}

func (s *OllaProxyService) GetStats(ctx context.Context) (ports.ProxyStats, error) {
	total := atomic.LoadInt64(&s.stats.totalRequests)
	successful := atomic.LoadInt64(&s.stats.successfulRequests)
	failed := atomic.LoadInt64(&s.stats.failedRequests)
	totalLatency := atomic.LoadInt64(&s.stats.totalLatency)

	var avgLatency int64
	if total > 0 {
		avgLatency = totalLatency / total
	}

	return ports.ProxyStats{
		TotalRequests:      total,
		SuccessfulRequests: successful,
		FailedRequests:     failed,
		AverageLatency:     avgLatency,
	}, nil
}

func (s *OllaProxyService) UpdateConfig(config ports.ProxyConfiguration) {
	s.configuration.ProxyPrefix = config.GetProxyPrefix()
	s.configuration.ResponseTimeout = config.GetResponseTimeout()
	s.configuration.ConnectionTimeout = config.GetConnectionTimeout()
}

func (s *OllaProxyService) updateStats(success bool, latency time.Duration) {
	atomic.AddInt64(&s.stats.totalRequests, 1)
	if success {
		atomic.AddInt64(&s.stats.successfulRequests, 1)
	} else {
		atomic.AddInt64(&s.stats.failedRequests, 1)
	}
	atomic.AddInt64(&s.stats.totalLatency, latency.Milliseconds())
}

func (s *OllaProxyService) recordFailure(endpoint *domain.Endpoint, duration time.Duration, bytes int64) {
	if endpoint != nil {
		s.statsCollector.RecordRequest(endpoint, "failure", duration, bytes)
	}
}
