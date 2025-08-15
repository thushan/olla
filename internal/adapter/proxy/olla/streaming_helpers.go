package olla

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/thushan/olla/internal/adapter/proxy/core"
	"github.com/thushan/olla/internal/logger"
)

// streamState manages the state during streaming
type streamState struct {
	disconnectTime       time.Time
	lastChunk            []byte
	totalBytes           int
	bytesAfterDisconnect int
	clientDisconnected   bool
}

// handleClientDisconnect processes client disconnection during streaming
func (s *Service) handleClientDisconnect(state *streamState, rlog logger.StyledLogger) {
	if !state.clientDisconnected {
		state.clientDisconnected = true
		state.disconnectTime = time.Now()
		rlog.Debug("client disconnected during streaming", "bytes_sent", state.totalBytes)

		// Publish client disconnect event
		s.PublishEvent(core.ProxyEvent{
			Type: core.EventTypeClientDisconnect,
			Metadata: core.ProxyEventMetadata{
				BytesSent: int64(state.totalBytes),
			},
		})
	}
}

// writeStreamData writes data to the response and handles flushing
func writeStreamData(w http.ResponseWriter, data []byte, canFlush bool, isStreaming bool, flusher http.Flusher) (int, error) {
	written, err := w.Write(data)
	if err != nil {
		return written, err
	}

	// Force data out for real-time streaming
	if canFlush && isStreaming {
		flusher.Flush()
	}

	return written, nil
}

// shouldStopAfterDisconnect checks if we should stop streaming after client disconnect
func shouldStopAfterDisconnect(bytesAfterDisconnect int, disconnectTime time.Time) bool {
	return bytesAfterDisconnect > ClientDisconnectionBytesThreshold ||
		time.Since(disconnectTime) > ClientDisconnectionTimeThreshold
}

// checkContexts checks for context cancellation and timeout
func (s *Service) checkContexts(clientCtx, upstreamCtx context.Context, readDeadline *time.Timer, state *streamState, rlog logger.StyledLogger) error {
	select {
	case <-clientCtx.Done():
		s.handleClientDisconnect(state, rlog)
		// Don't return error - let processStreamData handle client disconnect
		return nil
	case <-upstreamCtx.Done():
		return upstreamCtx.Err()
	case <-readDeadline.C:
		return fmt.Errorf("read timeout after %v", s.configuration.GetReadTimeout())
	default:
		return nil
	}
}

// processStreamData reads from upstream and writes to client
func (s *Service) processStreamData(resp *http.Response, buffer []byte, state *streamState, w http.ResponseWriter, canFlush bool, isStreaming bool, flusher http.Flusher, rlog logger.StyledLogger) error {
	n, err := resp.Body.Read(buffer)
	if n > 0 {
		// Only keep last chunk when we hit EOF (for metrics extraction)
		if errors.Is(err, io.EOF) {
			state.lastChunk = make([]byte, n)
			copy(state.lastChunk, buffer[:n])
		}

		// Handle data write
		if !state.clientDisconnected {
			written, writeErr := writeStreamData(w, buffer[:n], canFlush, isStreaming, flusher)
			state.totalBytes += written

			if writeErr != nil {
				rlog.Debug("write error during streaming", "error", writeErr, "bytes_written", state.totalBytes)
				return writeErr
			}
		} else {
			state.bytesAfterDisconnect += n

			if shouldStopAfterDisconnect(state.bytesAfterDisconnect, state.disconnectTime) {
				rlog.Debug("stopping stream after client disconnect",
					"bytes_after_disconnect", state.bytesAfterDisconnect,
					"time_since_disconnect", time.Since(state.disconnectTime))
				return context.Canceled
			}
		}
	}

	return err
}
