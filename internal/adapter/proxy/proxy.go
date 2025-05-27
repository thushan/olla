package proxy

import (
	"context"
	"fmt"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/internal/version"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

const (
	// TODO: add these to settings/config
	DefaultReadTimeout      = 60 * time.Second // Default read timeout for streaming responses
	DefaultStreamBufferSize = 8 * 1024         // 8 KB buffer for streaming responses

)

type SherpaProxyService struct {
	discoveryService ports.DiscoveryService
	selector         domain.EndpointSelector
	transport        *http.Transport
	configuration    *Configuration
	stats            *proxyStats
	logger           *logger.StyledLogger
}

type Configuration struct {
	ConnectionTimeout   time.Duration
	ConnectionKeepAlive time.Duration
	ResponseTimeout     time.Duration
	ReadTimeout         time.Duration
	ProxyPrefix         string
	StreamBufferSize    int
}

type proxyStats struct {
	totalRequests      int64
	successfulRequests int64
	failedRequests     int64
	totalLatency       int64
}

func NewService(
	discoveryService ports.DiscoveryService,
	selector domain.EndpointSelector,
	configuration *Configuration,
	logger *logger.StyledLogger,
) *SherpaProxyService {
	transport := &http.Transport{
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialer := &net.Dialer{
				Timeout:   configuration.ConnectionTimeout,
				KeepAlive: configuration.ConnectionKeepAlive,
			}
			conn, err := dialer.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			if tcpConn, ok := conn.(*net.TCPConn); ok {
				// TODO: Make this configurable to allow disabling Nagle's algorithm
				tcpConn.SetNoDelay(true)
			}
			return conn, nil
		},
	}

	return &SherpaProxyService{
		discoveryService: discoveryService,
		selector:         selector,
		transport:        transport,
		configuration:    configuration,
		stats:            &proxyStats{},
		logger:           logger,
	}
}

func (s *SherpaProxyService) ProxyRequest(ctx context.Context, w http.ResponseWriter, r *http.Request) (int, error) {
	atomic.AddInt64(&s.stats.totalRequests, 1)
	startTime := time.Now()
	var totalBytes = 0

	s.logger.Debug("proxy request started", "method", r.Method, "url", r.URL.String())

	endpoints, err := s.discoveryService.GetHealthyEndpoints(ctx)
	if err != nil {
		atomic.AddInt64(&s.stats.failedRequests, 1)
		s.logger.Error("failed to get healthy endpoints", "error", err)
		return totalBytes, fmt.Errorf("failed to get healthy endpoints: %w", err)
	}
	if len(endpoints) == 0 {
		atomic.AddInt64(&s.stats.failedRequests, 1)
		s.logger.Error("no healthy endpoints available")
		return totalBytes, fmt.Errorf("no healthy endpoints available - all endpoints may be down or still being health checked")
	}

	s.logger.Debug("found healthy endpoints", "count", len(endpoints))

	endpoint, err := s.selector.Select(ctx, endpoints)
	if err != nil {
		atomic.AddInt64(&s.stats.failedRequests, 1)
		s.logger.Error("failed to select endpoint", "error", err)
		return totalBytes, fmt.Errorf("failed to select endpoint: %w", err)
	}

	s.logger.Debug("selected endpoint", "url", endpoint.URL.String())

	// We have to strip the route prefix from the request path
	// to ensure we forward the correct path to the upstream service
	targetPath := s.stripRoutePrefix(r.Context(), r.URL.Path)
	targetURL, err := url.Parse(endpoint.URL.String() + targetPath)
	if err != nil {
		atomic.AddInt64(&s.stats.failedRequests, 1)
		s.logger.Error("failed to parse target URL", "error", err)
		return totalBytes, fmt.Errorf("failed to parse target URL: %w", err)
	}
	if r.URL.RawQuery != "" {
		targetURL.RawQuery = r.URL.RawQuery
	}

	s.logger.Debug("built target URL", "target", targetURL.String())

	// Create separate contexts for different phases

	// but we cache the original client context - detects client disconnections
	clientCtx := ctx

	// Create upstream context with response timeout
	// This context is independent of client disconnections to allow LLM responses to complete
	upstreamCtx := context.Background()
	if s.configuration.ResponseTimeout > 0 {
		var cancel context.CancelFunc
		upstreamCtx, cancel = context.WithTimeout(upstreamCtx, s.configuration.ResponseTimeout)
		defer cancel()
	}

	// Create proxy request with upstream context
	proxyReq, err := http.NewRequestWithContext(upstreamCtx, r.Method, targetURL.String(), r.Body)
	if err != nil {
		atomic.AddInt64(&s.stats.failedRequests, 1)
		s.logger.Error("failed to create proxy request", "error", err)
		return totalBytes, fmt.Errorf("failed to create proxy request: %w", err)
	}

	s.logger.Debug("created proxy request")

	// Copy headers and add forwarding headers
	s.copyHeaders(proxyReq, r)

	s.logger.Debug("making roundtrip request", "target", targetURL.String(), "time", time.Now())

	// Make the request using upstream context
	resp, err := s.transport.RoundTrip(proxyReq)
	if err != nil {
		atomic.AddInt64(&s.stats.failedRequests, 1)
		s.logger.Error("roundtrip failed", "error", err, "time", time.Now())
		return totalBytes, fmt.Errorf("upstream request failed: %w", err)
	}
	defer func(Body io.ReadCloser) {
		// this was a bit of a pain to hunt down when we had 40 servers running
		// leaks galore because we never closed the response body properly.
		_ = Body.Close()
	}(resp.Body)

	s.logger.Debug("roundtrip success", "status", resp.StatusCode, "time", time.Now())

	// Track connection only after successsful establishment
	s.selector.IncrementConnections(endpoint)
	defer s.selector.DecrementConnections(endpoint)

	// Copy response headers
	for k, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	s.logger.Debug("starting response stream", "time", time.Now())

	// Stream response using both contexts:
	// - clientCtx: detects when client disconnects
	// - upstreamCtx: controls upstream timeout behaviour
	// - resp.Body: the actual response stream
	if sumBytes, err := s.streamResponse(clientCtx, upstreamCtx, w, resp.Body); err != nil {
		atomic.AddInt64(&s.stats.failedRequests, 1)
		s.logger.Error("streaming failed", "error", err, "time", time.Now())
		return totalBytes, err
	} else {
		totalBytes = sumBytes
	}

	// Record success
	latency := time.Since(startTime).Milliseconds()
	atomic.AddInt64(&s.stats.successfulRequests, 1)
	atomic.AddInt64(&s.stats.totalLatency, latency)

	s.logger.Debug("proxy request completed", "latency_ms", latency, "time", time.Now())
	return totalBytes, nil
}

func (s *SherpaProxyService) stripRoutePrefix(ctx context.Context, path string) string {
	if prefix, ok := ctx.Value(s.configuration.ProxyPrefix).(string); ok {
		if strings.HasPrefix(path, prefix) {
			stripped := path[len(prefix):]
			if stripped == "" || stripped[0] != '/' {
				stripped = "/" + stripped
			}
			return stripped
		}
	}
	return path
}

func (s *SherpaProxyService) copyHeaders(proxyReq, originalReq *http.Request) {
	// c lone headers from the original request to the proxy request
	for k, vals := range originalReq.Header {
		for _, v := range vals {
			proxyReq.Header.Add(k, v)
		}
	}

	proto := "http"
	if originalReq.TLS != nil {
		proto = "https"
	}
	proxyReq.Header.Set("X-Forwarded-Host", originalReq.Host)
	proxyReq.Header.Set("X-Forwarded-Proto", proto)

	if ip, _, err := net.SplitHostPort(originalReq.RemoteAddr); err == nil {
		proxyReq.Header.Set("X-Forwarded-For", ip)
	}

	// being good netizens *RFC7230*
	proxyReq.Header.Set("X-Proxied-By", fmt.Sprintf("%s/%s", version.Name, version.Version))
	// proxyReq.Header.Set("X-Olla-Request-ID", generateRequestID())
	proxyReq.Header.Set("Via", fmt.Sprintf("1.1 olla/%s", version.Version))

	/*
		if endpoint := getEndpointFromContext(originalReq.Context()); endpoint != nil {
			proxyReq.Header.Set("X-Olla-Endpoint", endpoint.Name)
			proxyReq.Header.Set("X-Olla-Strategy", s.selector.Name())
		}
	*/
}

// streamResponse handles the complex streaming logic with dual context management
func (s *SherpaProxyService) streamResponse(clientCtx, upstreamCtx context.Context, w http.ResponseWriter, body io.Reader) (int, error) {
	buf := make([]byte, DefaultStreamBufferSize)

	flusher, canFlush := w.(http.Flusher)

	// Use read timeout for inter-chunk timeouts
	readTimeout := s.configuration.ReadTimeout
	if readTimeout == 0 {
		readTimeout = DefaultReadTimeout
	}

	s.logger.Debug("starting response stream", "read_timeout", readTimeout, "time", time.Now())

	totalBytes := 0
	readCount := 0
	lastReadTime := time.Now()

	for {
		// Check if client has disconnected (but don't let it cancel upstream immediately)
		select {
		case <-clientCtx.Done():
			s.logger.Info("client disconnected during streaming",
				"total_bytes", totalBytes,
				"read_count", readCount,
				"error", clientCtx.Err(),
				"time", time.Now())
			// For LLM responses, we might want to continue for a short time
			// in case the client reconnects or it's a temporary network issue
			if s.shouldContinueAfterClientDisconnect(totalBytes, time.Since(lastReadTime)) {
				s.logger.Debug("continuing stream briefly after client disconnect")
				// Continue but with a shorter timeout
				clientCtx = context.Background()
			} else {
				return totalBytes, fmt.Errorf("client disconnected: %w", clientCtx.Err())
			}
		default:
			// Client still connected, carry on son
		}

		// Check upstream timeout
		select {
		case <-upstreamCtx.Done():
			s.logger.Error("upstream timeout exceeded",
				"total_bytes", totalBytes,
				"read_count", readCount,
				"error", upstreamCtx.Err(),
				"time", time.Now())
			return totalBytes, fmt.Errorf("upstream timeout: %w", upstreamCtx.Err())
		default:
			// Upstream still valid, continue
		}

		// Use a separate goroutine for reading to handle multiple context cancellations
		type readResult struct {
			n   int
			err error
		}

		readCh := make(chan readResult, 1)
		readStart := time.Now()

		go func() {
			n, err := body.Read(buf)
			readCh <- readResult{n: n, err: err}
		}()

		// Wait for read with timeout
		readTimer := time.NewTimer(readTimeout)

		select {
		case <-clientCtx.Done():
			readTimer.Stop()
			s.logger.Debug("client context cancelled during read wait",
				"total_bytes", totalBytes,
				"read_count", readCount,
				"time", time.Now())
			// Don't immediately return - let the read complete and then decide
			select {
			case result := <-readCh:
				if result.n > 0 {
					// We got data, try to send it
					if _, err := w.Write(buf[:result.n]); err != nil {
						return totalBytes, fmt.Errorf("failed to write response after client disconnect: %w", err)
					}
					if canFlush {
						flusher.Flush()
					}
				}
			case <-time.After(1 * time.Second):
				// Read is taking too long after client disconnect
			}
			return totalBytes, fmt.Errorf("client disconnected: %w", clientCtx.Err())

		case <-upstreamCtx.Done():
			readTimer.Stop()
			s.logger.Error("upstream context cancelled during read wait",
				"total_bytes", totalBytes,
				"read_count", readCount,
				"time", time.Now())
			return totalBytes, fmt.Errorf("upstream timeout during read: %w", upstreamCtx.Err())

		case <-readTimer.C:
			s.logger.Error("read timeout exceeded between chunks",
				"timeout", readTimeout,
				"total_bytes", totalBytes,
				"read_count", readCount,
				"time_since_last_read", time.Since(lastReadTime),
				"time", time.Now())
			return totalBytes, fmt.Errorf("no response data received within %v", readTimeout)

		case result := <-readCh:
			readTimer.Stop()
			readDuration := time.Since(readStart)

			if result.n > 0 {
				totalBytes += result.n
				readCount++
				lastReadTime = time.Now()

				s.logger.Debug("stream read success",
					"read_num", readCount,
					"bytes", result.n,
					"duration_ms", readDuration.Milliseconds(),
					"total_bytes", totalBytes,
					"time", time.Now())

				if _, err := w.Write(buf[:result.n]); err != nil {
					s.logger.Error("failed to write response", "error", err, "time", time.Now())
					return totalBytes, fmt.Errorf("failed to write response: %w", err)
				}
				if canFlush {
					flusher.Flush()
				}
			} else if result.n == 0 && result.err == nil {
				s.logger.Debug("empty read", "read_num", readCount+1, "duration_ms", readDuration.Milliseconds())
			}

			if result.err != nil {
				if result.err == io.EOF {
					s.logger.Debug("stream ended normally",
						"total_bytes", totalBytes,
						"read_count", readCount,
						"time", time.Now())
					return totalBytes, nil // Normal end of stream
				}
				s.logger.Error("stream read error",
					"error", result.err,
					"total_bytes", totalBytes,
					"read_count", readCount,
					"time", time.Now())
				return totalBytes, fmt.Errorf("failed to read response: %w", result.err)
			}
		}
	}
}

// shouldContinueAfterClientDisconnect determines if we should continue streaming
// after client disconnect - useful for LLM responses that might be valuable to cache
func (s *SherpaProxyService) shouldContinueAfterClientDisconnect(bytesRead int, timeSinceLastRead time.Duration) bool {
	// Continue if we've already read significant data and the stream is still active
	// This allows for brief network interruptions during long LLM responses
	return bytesRead > 1024 && timeSinceLastRead < 5*time.Second
}

func (s *SherpaProxyService) GetStats(ctx context.Context) (ports.ProxyStats, error) {
	total := atomic.LoadInt64(&s.stats.totalRequests)
	successful := atomic.LoadInt64(&s.stats.successfulRequests)
	failed := atomic.LoadInt64(&s.stats.failedRequests)
	totalLatency := atomic.LoadInt64(&s.stats.totalLatency)

	var avgLatency int64
	if successful > 0 {
		avgLatency = totalLatency / successful
	}

	return ports.ProxyStats{
		TotalRequests:      total,
		SuccessfulRequests: successful,
		FailedRequests:     failed,
		AverageLatency:     avgLatency,
	}, nil
}

func (s *SherpaProxyService) UpdateConfig(configuration *Configuration) {
	s.configuration = configuration
}
