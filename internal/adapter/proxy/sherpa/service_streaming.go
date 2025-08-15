package sherpa

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/thushan/olla/internal/core/constants"

	"github.com/thushan/olla/internal/adapter/proxy/core"

	"github.com/thushan/olla/internal/adapter/proxy/common"
	"github.com/thushan/olla/internal/logger"
)

// streamState tracks the state of an active stream
type streamState struct {
	lastReadTime         time.Time
	contentType          string
	lastChunk            []byte // Store last chunk for metrics extraction
	totalBytes           int
	readCount            int
	bytesAfterDisconnect int
	clientDisconnected   bool
	isStreaming          bool
}

// readResult contains the result of a read operation
type readResult struct {
	err error
	n   int
}

// streamResponseWithTimeout performs buffered streaming with read timeout protection
// This is CRITICAL for edge servers to prevent hanging on unresponsive backends (SHERPA-105)
func (s *Service) streamResponseWithTimeout(clientCtx, upstreamCtx context.Context, w http.ResponseWriter, resp *http.Response, buffer []byte, rlog logger.StyledLogger) (int, []byte, error) {
	state := &streamState{
		lastReadTime: time.Now(),
		contentType:  resp.Header.Get(constants.HeaderContentType),
		isStreaming:  core.AutoDetectStreamingMode(clientCtx, resp, s.configuration.GetProxyProfile()),
	}

	readTimeout := s.getReadTimeout()
	rlog.Debug("starting response stream", "read_timeout", readTimeout, "buffer_size", len(buffer), "profile", s.configuration.GetProxyProfile())

	// Create combined context for cancellation
	combinedCtx, cancel := s.createCombinedContext(clientCtx, upstreamCtx)
	defer cancel()

	flusher, canFlush := w.(http.Flusher)
	if !canFlush {
		rlog.Warn("ResponseWriter does not support flushing - streaming may be buffered")
	}

	for {
		result, err := s.performTimedRead(combinedCtx, resp.Body, buffer, readTimeout, state, rlog)
		if err != nil {
			return state.totalBytes, state.lastChunk, err
		}

		if result == nil {
			// Context cancelled during read
			return state.totalBytes, state.lastChunk, s.handleContextCancellation(clientCtx, upstreamCtx, state, rlog)
		}

		// Process read result
		done, err := s.processReadResult(result, w, buffer, flusher, canFlush, state, rlog)
		if done || err != nil {
			return state.totalBytes, state.lastChunk, err
		}
	}
}

// getReadTimeout returns the configured read timeout or default
func (s *Service) getReadTimeout() time.Duration {
	readTimeout := s.configuration.GetReadTimeout()
	if readTimeout == 0 {
		readTimeout = DefaultReadTimeout
	}
	return readTimeout
}

// createCombinedContext creates a context that cancels when either client or upstream context is done
func (s *Service) createCombinedContext(clientCtx, upstreamCtx context.Context) (context.Context, context.CancelFunc) {
	combinedCtx, cancel := context.WithCancel(context.Background())

	go func() {
		select {
		case <-clientCtx.Done():
			cancel()
		case <-upstreamCtx.Done():
			cancel()
		case <-combinedCtx.Done():
			return // Exit when combined context is cancelled
		}
	}()

	return combinedCtx, cancel
}

// performTimedRead performs a read operation with timeout
func (s *Service) performTimedRead(combinedCtx context.Context, body io.Reader, buffer []byte, readTimeout time.Duration, state *streamState, rlog logger.StyledLogger) (*readResult, error) {
	readCh := make(chan readResult, 1)
	readStart := time.Now()

	// spawn goroutine for read operation
	go func() {
		n, err := body.Read(buffer)
		select {
		case readCh <- readResult{n: n, err: err}:
			// Successfully sent result
		case <-combinedCtx.Done():
			// Context cancelled, exit without blocking
		}
	}()

	// Set timer to detect stalled reads
	readTimer := time.NewTimer(readTimeout)
	defer readTimer.Stop()

	select {
	case <-combinedCtx.Done():
		// Context cancelled - try to complete current read
		gracePeriod := time.NewTimer(1 * time.Second)
		defer gracePeriod.Stop()

		select {
		case result := <-readCh:
			if result.n > 0 && !state.clientDisconnected {
				// Return result to be processed
				return &result, nil
			}
		case <-gracePeriod.C:
			// Give up on pending read
		}
		return nil, nil // Signals context cancellation

	case <-readTimer.C:
		// Read timeout - critical for detecting stalled backends
		rlog.Error("read timeout exceeded between chunks",
			"timeout", readTimeout,
			"total_bytes", state.totalBytes,
			"read_count", state.readCount,
			"time_since_last_read", time.Since(state.lastReadTime))
		return nil, fmt.Errorf("AI backend stopped responding - no data received for %.1fs (backend may be overloaded)", readTimeout.Seconds())

	case result := <-readCh:
		state.readCount++
		state.lastReadTime = time.Now()
		s.logReadMetrics(result.n, time.Since(readStart), state, rlog)
		return &result, nil
	}
}

// handleContextCancellation handles client or upstream context cancellation
func (s *Service) handleContextCancellation(clientCtx, upstreamCtx context.Context, state *streamState, rlog logger.StyledLogger) error {
	if clientCtx.Err() != nil {
		if !state.clientDisconnected {
			state.clientDisconnected = true
			rlog.Info("client disconnected during streaming",
				"total_bytes", state.totalBytes,
				"read_count", state.readCount)
		}
		return context.Canceled
	}

	if upstreamCtx.Err() != nil {
		rlog.Error("upstream context cancelled",
			"total_bytes", state.totalBytes,
			"read_count", state.readCount)
		return common.MakeUserFriendlyError(upstreamCtx.Err(), time.Since(state.lastReadTime), "streaming", s.configuration.GetResponseTimeout())
	}

	return nil
}

// processReadResult processes the result of a read operation
func (s *Service) processReadResult(result *readResult, w http.ResponseWriter, buffer []byte, flusher http.Flusher, canFlush bool, state *streamState, rlog logger.StyledLogger) (bool, error) {
	n, err := result.n, result.err

	// Handle data if available
	if n > 0 {
		if writeErr := s.writeData(w, buffer[:n], flusher, canFlush, state, rlog); writeErr != nil {
			return true, writeErr
		}
	} else if n == 0 && err == nil {
		rlog.Debug("empty read", "read_count", state.readCount)
	}

	// Handle errors
	if err != nil {
		if errors.Is(err, io.EOF) {
			rlog.Debug("stream ended normally",
				"total_bytes", state.totalBytes,
				"read_count", state.readCount)
			return true, nil
		}
		rlog.Error("stream read error",
			"error", err,
			"total_bytes", state.totalBytes,
			"read_count", state.readCount)
		return true, err
	}

	return false, nil
}

// writeData writes data to the response writer
func (s *Service) writeData(w http.ResponseWriter, data []byte, flusher http.Flusher, canFlush bool, state *streamState, rlog logger.StyledLogger) error {
	// Store chunk for potential metrics extraction using a ring buffer approach
	// Keep only last 8KB to avoid excessive memory usage while still capturing metrics
	const maxLastChunkSize = 8192

	if len(data) > 0 {
		if len(data) <= maxLastChunkSize {
			// Data fits entirely, just replace lastChunk
			if state.lastChunk == nil || cap(state.lastChunk) < len(data) {
				state.lastChunk = make([]byte, len(data))
			} else {
				state.lastChunk = state.lastChunk[:len(data)]
			}
			copy(state.lastChunk, data)
		} else {
			// Data is larger than max, keep only the last portion
			if state.lastChunk == nil || cap(state.lastChunk) < maxLastChunkSize {
				state.lastChunk = make([]byte, maxLastChunkSize)
			} else {
				state.lastChunk = state.lastChunk[:maxLastChunkSize]
			}
			copy(state.lastChunk, data[len(data)-maxLastChunkSize:])
		}
	}

	if !state.clientDisconnected {
		written, writeErr := w.Write(data)
		state.totalBytes += written

		if writeErr != nil {
			rlog.Error("failed to write response", "error", writeErr)
			return writeErr
		}

		// Flush if we're in streaming mode
		if canFlush && s.shouldFlush(state) {
			flusher.Flush()
		}
	} else {
		// Track bytes after disconnect
		// client has gone gone, but we'll finish the current chunk, chunky stuff!
		state.bytesAfterDisconnect += len(data)
		rlog.Debug("continuing stream briefly after client disconnect")

		if state.bytesAfterDisconnect > ClientDisconnectionBytesThreshold {
			rlog.Debug("stopping stream after client disconnect",
				"bytes_after_disconnect", state.bytesAfterDisconnect)
			return context.Canceled
		}
	}
	return nil
}

// logReadMetrics logs metrics for read operations
func (s *Service) logReadMetrics(bytesRead int, duration time.Duration, state *streamState, rlog logger.StyledLogger) {
	if bytesRead > 0 && (state.readCount > 1 || duration > 100*time.Millisecond) {
		rlog.Debug("stream read success",
			"bytes", bytesRead,
			"duration_ms", duration.Milliseconds(),
			"read_count", state.readCount,
			"total_bytes", state.totalBytes)
	}
}
