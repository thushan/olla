package metrics

import (
	"testing"
	"time"

	"github.com/thushan/olla/internal/core/ports"
)

func TestRequestCollector_RecordAndRetrieve(t *testing.T) {
	c := NewRequestCollectorWithConfig(10, 10)
	defer c.Shutdown()

	now := time.Now()
	for i := 0; i < 5; i++ {
		c.RecordRequestMetrics(ports.RequestMetricsEvent{
			StartTime:       now.Add(time.Duration(i) * time.Second),
			EndTime:         now.Add(time.Duration(i)*time.Second + 100*time.Millisecond),
			Model:           "test-model",
			EndpointName:    "ep-1",
			TTFTMs:          int64(50 + i*10),
			TotalDurationMs: 100,
			InputTokens:     10,
			OutputTokens:    20,
			Success:         true,
			IsStreaming:      true,
		})
	}

	// Allow consumer goroutine to process
	time.Sleep(50 * time.Millisecond)

	recent := c.GetRecentRequests(3)
	if len(recent) != 3 {
		t.Fatalf("expected 3 recent, got %d", len(recent))
	}

	// Most recent first
	if recent[0].TTFTMs != 90 {
		t.Errorf("expected most recent TTFT=90, got %d", recent[0].TTFTMs)
	}
}

func TestRequestCollector_RingBufferWraparound(t *testing.T) {
	c := NewRequestCollectorWithConfig(3, 10)
	defer c.Shutdown()

	now := time.Now()
	for i := 0; i < 5; i++ {
		c.RecordRequestMetrics(ports.RequestMetricsEvent{
			StartTime:       now.Add(time.Duration(i) * time.Second),
			Model:           "model",
			TotalDurationMs: int64(100 + i),
			Success:         true,
		})
	}

	time.Sleep(50 * time.Millisecond)

	recent := c.GetRecentRequests(10)
	if len(recent) != 3 {
		t.Fatalf("expected 3 (ring cap), got %d", len(recent))
	}

	// Should have the last 3 entries (duration 102, 103, 104)
	if recent[0].TotalDurationMs != 104 {
		t.Errorf("expected duration=104, got %d", recent[0].TotalDurationMs)
	}
}

func TestRequestCollector_AggregatedStats(t *testing.T) {
	c := NewRequestCollectorWithConfig(100, 100)
	defer c.Shutdown()

	now := time.Now()
	for i := 0; i < 10; i++ {
		c.RecordRequestMetrics(ports.RequestMetricsEvent{
			StartTime:       now.Add(time.Duration(i) * time.Second),
			EndTime:         now.Add(time.Duration(i)*time.Second + 200*time.Millisecond),
			Model:           "test-model",
			EndpointName:    "ep-1",
			TTFTMs:          int64(100 + i*10),
			TotalDurationMs: 200,
			InputTokens:     10,
			OutputTokens:    50,
			TokensPerSecond: 250.0,
			Success:         true,
			IsStreaming:      true,
		})
	}

	time.Sleep(50 * time.Millisecond)

	stats := c.GetAggregatedStats(time.Time{})
	if stats.TotalRequests != 10 {
		t.Fatalf("expected 10 requests, got %d", stats.TotalRequests)
	}
	if stats.SuccessfulRequests != 10 {
		t.Errorf("expected 10 successful, got %d", stats.SuccessfulRequests)
	}
	if stats.TotalInputTokens != 100 {
		t.Errorf("expected 100 input tokens, got %d", stats.TotalInputTokens)
	}
	if stats.TotalOutputTokens != 500 {
		t.Errorf("expected 500 output tokens, got %d", stats.TotalOutputTokens)
	}
	if stats.TTFTAvgMs == 0 {
		t.Error("expected non-zero TTFT avg")
	}
	if stats.AvgTokensPerSec == 0 {
		t.Error("expected non-zero avg tokens/s")
	}

	// Check per-model breakdown
	ms, ok := stats.ByModel["test-model"]
	if !ok {
		t.Fatal("expected per-model stats for test-model")
	}
	if ms.TotalRequests != 10 {
		t.Errorf("expected 10 per-model requests, got %d", ms.TotalRequests)
	}
}

func TestStreamTap_TTFT(t *testing.T) {
	start := time.Now()
	tap := NewStreamTap(start)

	// Simulate delay then SSE data
	time.Sleep(10 * time.Millisecond)
	tap.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"))

	ttft := tap.TTFT()
	if ttft < 10 {
		t.Errorf("expected TTFT >= 10ms, got %d", ttft)
	}
	if !tap.HasReceivedData() {
		t.Error("expected HasReceivedData to be true")
	}
}

func TestStreamTap_NoSSEPrefix(t *testing.T) {
	start := time.Now()
	tap := NewStreamTap(start)

	// Write non-SSE data
	tap.Write([]byte("HTTP/1.1 200 OK\r\n"))

	// First byte recorded
	if !tap.HasReceivedData() {
		t.Error("expected HasReceivedData to be true")
	}

	ttft := tap.TTFT()
	if ttft == 0 {
		t.Error("expected non-zero TTFT for first byte")
	}
}

func TestPercentile(t *testing.T) {
	values := []int64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}

	p50 := percentile(values, 50)
	if p50 != 60 {
		t.Errorf("expected p50=60, got %d", p50)
	}

	p95 := percentile(values, 95)
	if p95 != 100 {
		t.Errorf("expected p95=100, got %d", p95)
	}

	p99 := percentile(values, 99)
	if p99 != 100 {
		t.Errorf("expected p99=100, got %d", p99)
	}
}
