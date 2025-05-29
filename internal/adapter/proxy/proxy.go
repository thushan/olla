package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/internal/version"
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

const (
	// TODO: add these to settings/config
	DefaultReadTimeout                = 60 * time.Second
	DefaultStreamBufferSize           = 8 * 1024
	DefaultSetNoDelay                 = true
	DefaultTimeout                    = 60 * time.Second
	DefaultKeepAlive                  = 60 * time.Second
	DefaultMaxIdleConns               = 100
	DefaultIdleConnTimeout            = 90 * time.Second
	DefaultTLSHandshakeTimeout        = 10 * time.Second
	ClientDisconnectionBytesThreshold = 1024
	ClientDisconnectionTimeThreshold  = 5 * time.Second
)

func NewService(
	discoveryService ports.DiscoveryService,
	selector domain.EndpointSelector,
	configuration *Configuration,
	logger *logger.StyledLogger,
) *SherpaProxyService {
	transport := &http.Transport{
		MaxIdleConns:        DefaultMaxIdleConns,
		IdleConnTimeout:     DefaultIdleConnTimeout,
		TLSHandshakeTimeout: DefaultTLSHandshakeTimeout,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialer := &net.Dialer{
				Timeout:   DefaultTimeout,
				KeepAlive: DefaultKeepAlive,
			}
			conn, err := dialer.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			if tcpConn, ok := conn.(*net.TCPConn); ok {
				if terr := tcpConn.SetNoDelay(DefaultSetNoDelay); terr != nil {
					logger.Warn("Failed to set NoDelay", "err", terr)
				}
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

	// Strip route prefix from request path for upstream
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

	// Create upstream context with response timeout
	upstreamCtx := ctx
	if s.configuration.ResponseTimeout > 0 {
		var cancel context.CancelFunc
		upstreamCtx, cancel = context.WithTimeout(ctx, s.configuration.ResponseTimeout)
		defer cancel()
	}

	proxyReq, err := http.NewRequestWithContext(upstreamCtx, r.Method, targetURL.String(), r.Body)
	if err != nil {
		atomic.AddInt64(&s.stats.failedRequests, 1)
		s.logger.Error("failed to create proxy request", "error", err)
		return totalBytes, fmt.Errorf("failed to create proxy request: %w", err)
	}

	s.logger.Debug("created proxy request")

	s.copyHeaders(proxyReq, r)

	s.logger.Debug("making roundtrip request", "target", targetURL.String(), "time", time.Now())

	resp, err := s.transport.RoundTrip(proxyReq)
	if err != nil {
		atomic.AddInt64(&s.stats.failedRequests, 1)
		s.logger.Error("roundtrip failed", "error", err, "time", time.Now())
		return totalBytes, fmt.Errorf("upstream request failed: %w", err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	s.logger.Debug("roundtrip success", "status", resp.StatusCode, "time", time.Now())

	// Track connection after successful establishment
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

	if sumBytes, err := s.streamResponse(ctx, upstreamCtx, w, resp.Body); err != nil {
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
	proxyReq.Header.Set("Via", fmt.Sprintf("1.1 olla/%s", version.Version))
}

func (s *SherpaProxyService) streamResponse(clientCtx, upstreamCtx context.Context, w http.ResponseWriter, body io.Reader) (int, error) {
	bufferSize := s.configuration.StreamBufferSize
	if bufferSize == 0 {
		bufferSize = DefaultStreamBufferSize
	}
	buf := make([]byte, bufferSize)

	flusher, canFlush := w.(http.Flusher)

	readTimeout := s.configuration.ReadTimeout
	if readTimeout == 0 {
		readTimeout = DefaultReadTimeout
	}

	s.logger.Debug("starting response stream", "read_timeout", readTimeout, "time", time.Now())

	totalBytes := 0
	readCount := 0
	lastReadTime := time.Now()

	for {
		// Check if client disconnected
		select {
		case <-clientCtx.Done():
			s.logger.Info("client disconnected during streaming",
				"total_bytes", totalBytes,
				"read_count", readCount,
				"error", clientCtx.Err(),
				"time", time.Now())
			// Continue briefly for LLM responses in case client reconnects
			if s.shouldContinueAfterClientDisconnect(totalBytes, time.Since(lastReadTime)) {
				s.logger.Debug("continuing stream briefly after client disconnect")
				clientCtx = context.Background()
			} else {
				return totalBytes, fmt.Errorf("client disconnected: %w", clientCtx.Err())
			}
		default:
			// Client still connected
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
			// Upstream still valid
		}

		// Read with timeout handling
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

		readTimer := time.NewTimer(readTimeout)

		select {
		case <-clientCtx.Done():
			readTimer.Stop()
			s.logger.Debug("client context cancelled during read wait",
				"total_bytes", totalBytes,
				"read_count", readCount,
				"time", time.Now())
			// Try to complete current read and send it
			select {
			case result := <-readCh:
				if result.n > 0 {
					if _, err := w.Write(buf[:result.n]); err != nil {
						return totalBytes, fmt.Errorf("failed to write response after client disconnect: %w", err)
					}
					if canFlush {
						flusher.Flush()
					}
				}
			case <-time.After(1 * time.Second):
				// Read taking too long after client disconnect
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
					return totalBytes, nil
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

func (s *SherpaProxyService) shouldContinueAfterClientDisconnect(bytesRead int, timeSinceLastRead time.Duration) bool {
	// Continue if we've read significant data and stream is still active
	// Allows for brief network interruptions during long LLM responses
	return bytesRead > ClientDisconnectionBytesThreshold && timeSinceLastRead < ClientDisconnectionTimeThreshold
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
