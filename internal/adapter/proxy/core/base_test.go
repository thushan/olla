package core

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/thushan/olla/internal/adapter/stats"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

func createBaseTestLogger() logger.StyledLogger {
	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, _ := logger.New(loggerCfg)
	return logger.NewPlainStyledLogger(log)
}

func newBaseTestComponents() (*BaseProxyComponents, *stats.Collector) {
	collector := stats.NewCollector(createBaseTestLogger())
	return &BaseProxyComponents{StatsCollector: collector}, collector
}

func newBaseTestEndpoint() *domain.Endpoint {
	u, _ := url.Parse("http://localhost:11434")
	return &domain.Endpoint{
		Name:      "ep1",
		URL:       u,
		URLString: u.String(),
		Status:    domain.StatusHealthy,
	}
}

// TestBaseProxyComponents_RecordSuccess_WithModelName is the regression test
// for RecordModelRequest's dead-since-ed93e9a callers: a successful request
// with a resolved model name must populate the model-level stats, not just
// the per-endpoint ones, so /internal/stats/models has data to report.
func TestBaseProxyComponents_RecordSuccess_WithModelName(t *testing.T) {
	b, collector := newBaseTestComponents()
	endpoint := newBaseTestEndpoint()

	b.RecordSuccess(endpoint, "llama3.2", 42, 1024)

	modelStats := collector.GetModelStats()
	ms, ok := modelStats["llama3.2"]
	if !ok {
		t.Fatalf("expected model stats for llama3.2, got %v", modelStats)
	}
	if ms.TotalRequests != 1 || ms.SuccessfulRequests != 1 {
		t.Errorf("expected 1 total/successful request, got %+v", ms)
	}
	if ms.TotalBytes != 1024 {
		t.Errorf("expected 1024 total bytes, got %d", ms.TotalBytes)
	}
}

// TestBaseProxyComponents_RecordFailure_WithModelName mirrors the success
// case for the failure path.
func TestBaseProxyComponents_RecordFailure_WithModelName(t *testing.T) {
	b, collector := newBaseTestComponents()
	endpoint := newBaseTestEndpoint()

	b.RecordFailure(context.Background(), endpoint, "llama3.2", 15*time.Millisecond, errors.New("boom"))

	modelStats := collector.GetModelStats()
	ms, ok := modelStats["llama3.2"]
	if !ok {
		t.Fatalf("expected model stats for llama3.2, got %v", modelStats)
	}
	if ms.TotalRequests != 1 || ms.FailedRequests != 1 {
		t.Errorf("expected 1 total/failed request, got %+v", ms)
	}
}

// TestBaseProxyComponents_RecordSuccess_EmptyModelNameSkipsModelStats guards
// the modelName != "" gate: requests where the model couldn't be resolved
// (e.g. no endpoints available) must not create a spurious "" entry in
// model-level stats, matching the deleted proxy_olla.go's original guard.
func TestBaseProxyComponents_RecordSuccess_EmptyModelNameSkipsModelStats(t *testing.T) {
	b, collector := newBaseTestComponents()
	endpoint := newBaseTestEndpoint()

	b.RecordSuccess(endpoint, "", 42, 1024)

	if _, ok := collector.GetModelStats()[""]; ok {
		t.Error("empty model name must not create a model-stats entry")
	}
	// The per-endpoint stats must still be recorded regardless.
	endpointStats := collector.GetEndpointStats()
	if _, ok := endpointStats[endpoint.URLString]; !ok {
		t.Error("expected per-endpoint stats to be recorded even without a model name")
	}
}

// TestBaseProxyComponents_RecordFailure_EmptyModelNameSkipsModelStats mirrors
// the guard test for the failure path.
func TestBaseProxyComponents_RecordFailure_EmptyModelNameSkipsModelStats(t *testing.T) {
	b, collector := newBaseTestComponents()

	b.RecordFailure(context.Background(), nil, "", 5*time.Millisecond, errors.New("no healthy endpoints"))

	if _, ok := collector.GetModelStats()[""]; ok {
		t.Error("empty model name must not create a model-stats entry")
	}
}
