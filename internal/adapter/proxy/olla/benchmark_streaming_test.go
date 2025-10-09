package olla

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// BenchmarkStreamingLastChunk benchmarks the streaming last chunk capture with and without pre-allocated buffer.
// This demonstrates the allocation reduction from using a pre-allocated buffer in streamState.
func BenchmarkStreamingLastChunk(b *testing.B) {
	// Simulate a typical streaming response with EOF on the last chunk
	// Pre-create streamState to measure only the EOF handling allocation
	b.Run("PreAllocated_SmallChunk", func(b *testing.B) {
		buffer := make([]byte, 8192)
		data := []byte(`{"choices":[{"delta":{"content":"test"},"finish_reason":"stop"}]}`)
		w := httptest.NewRecorder()
		state := &streamState{} // Allocate once outside the loop

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			resp := &http.Response{
				Body: io.NopCloser(bytes.NewReader(data)),
			}

			// Simulate EOF read (last chunk scenario)
			n, err := resp.Body.Read(buffer)
			if n > 0 {
				if errors.Is(err, io.EOF) {
					if n <= len(state.lastChunkBuf) {
						copy(state.lastChunkBuf[:], buffer[:n])
						state.lastChunk = state.lastChunkBuf[:n]
					} else {
						state.lastChunk = make([]byte, n)
						copy(state.lastChunk, buffer[:n])
					}
				}
				w.Write(buffer[:n])
			}
		}
	})

	b.Run("PreAllocated_LargeChunk", func(b *testing.B) {
		buffer := make([]byte, 8192)
		// 4KB chunk - typical size
		data := make([]byte, 4096)
		for i := range data {
			data[i] = byte(i % 256)
		}
		w := httptest.NewRecorder()
		state := &streamState{} // Allocate once outside the loop

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			resp := &http.Response{
				Body: io.NopCloser(bytes.NewReader(data)),
			}

			n, err := resp.Body.Read(buffer)
			if n > 0 {
				if errors.Is(err, io.EOF) {
					if n <= len(state.lastChunkBuf) {
						copy(state.lastChunkBuf[:], buffer[:n])
						state.lastChunk = state.lastChunkBuf[:n]
					} else {
						state.lastChunk = make([]byte, n)
						copy(state.lastChunk, buffer[:n])
					}
				}
				w.Write(buffer[:n])
			}
		}
	})

	b.Run("PreAllocated_OversizedChunk", func(b *testing.B) {
		buffer := make([]byte, 16384) // Larger buffer for this test
		// 12KB chunk - exceeds pre-allocated buffer (rare case)
		data := make([]byte, 12288)
		for i := range data {
			data[i] = byte(i % 256)
		}
		w := httptest.NewRecorder()
		state := &streamState{} // Allocate once outside the loop

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			resp := &http.Response{
				Body: io.NopCloser(bytes.NewReader(data)),
			}

			n, err := resp.Body.Read(buffer)
			if n > 0 {
				if errors.Is(err, io.EOF) {
					if n <= len(state.lastChunkBuf) {
						copy(state.lastChunkBuf[:], buffer[:n])
						state.lastChunk = state.lastChunkBuf[:n]
					} else {
						// Fallback allocation for oversized chunks
						state.lastChunk = make([]byte, n)
						copy(state.lastChunk, buffer[:n])
					}
				}
				w.Write(buffer[:n])
			}
		}
	})

	// Comparison: Old allocation pattern (for reference)
	b.Run("OldAllocation_SmallChunk", func(b *testing.B) {
		buffer := make([]byte, 8192)
		data := []byte(`{"choices":[{"delta":{"content":"test"},"finish_reason":"stop"}]}`)
		w := httptest.NewRecorder()

		// Create a mock state without pre-allocated buffer (simulating old code)
		type oldStreamState struct {
			lastChunk []byte
		}
		state := &oldStreamState{}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			resp := &http.Response{
				Body: io.NopCloser(bytes.NewReader(data)),
			}

			n, err := resp.Body.Read(buffer)
			if n > 0 {
				if errors.Is(err, io.EOF) {
					// Old pattern: always allocate
					state.lastChunk = make([]byte, n)
					copy(state.lastChunk, buffer[:n])
				}
				w.Write(buffer[:n])
			}
		}
	})

	b.Run("OldAllocation_LargeChunk", func(b *testing.B) {
		buffer := make([]byte, 8192)
		data := make([]byte, 4096)
		for i := range data {
			data[i] = byte(i % 256)
		}
		w := httptest.NewRecorder()

		type oldStreamState struct {
			lastChunk []byte
		}
		state := &oldStreamState{}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			resp := &http.Response{
				Body: io.NopCloser(bytes.NewReader(data)),
			}

			n, err := resp.Body.Read(buffer)
			if n > 0 {
				if errors.Is(err, io.EOF) {
					// Old pattern: always allocate
					state.lastChunk = make([]byte, n)
					copy(state.lastChunk, buffer[:n])
				}
				w.Write(buffer[:n])
			}
		}
	})
}
