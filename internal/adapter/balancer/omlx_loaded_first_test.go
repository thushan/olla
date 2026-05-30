package balancer

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
)

func TestOMLXLoadedFirstSelector_PrefersLoadedModel(t *testing.T) {
	selector := NewOMLXLoadedFirstSelector(NewTestStatsCollector())
	ctx := context.WithValue(context.Background(), constants.ContextModelKey, "target-model")

	coldEndpoint, coldCleanup := makeOMLXStatusEndpoint(t, "cold", map[string]bool{"target-model": false})
	defer coldCleanup()
	loadedEndpoint, loadedCleanup := makeOMLXStatusEndpoint(t, "loaded", map[string]bool{"target-model": true})
	defer loadedCleanup()
	storeOMLXStatus(selector, coldEndpoint, map[string]bool{"target-model": false})
	storeOMLXStatus(selector, loadedEndpoint, map[string]bool{"target-model": true})

	selected, err := selector.Select(ctx, []*domain.Endpoint{coldEndpoint, loadedEndpoint})
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}

	if selected.Name != "loaded" {
		t.Fatalf("expected loaded endpoint, got %s", selected.Name)
	}
}

func TestOMLXLoadedFirstSelector_FallsBackToLeastConnections(t *testing.T) {
	selector := NewOMLXLoadedFirstSelector(NewTestStatsCollector())
	ctx := context.WithValue(context.Background(), constants.ContextModelKey, "target-model")

	busyEndpoint, busyCleanup := makeOMLXStatusEndpoint(t, "busy", map[string]bool{"target-model": false})
	defer busyCleanup()
	idleEndpoint, idleCleanup := makeOMLXStatusEndpoint(t, "idle", map[string]bool{"target-model": false})
	defer idleCleanup()
	storeOMLXStatus(selector, busyEndpoint, map[string]bool{"target-model": false})
	storeOMLXStatus(selector, idleEndpoint, map[string]bool{"target-model": false})

	selector.IncrementConnections(busyEndpoint)

	selected, err := selector.Select(ctx, []*domain.Endpoint{busyEndpoint, idleEndpoint})
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}

	if selected.Name != "idle" {
		t.Fatalf("expected least-connected fallback endpoint, got %s", selected.Name)
	}
}

func TestOMLXLoadedFirstSelector_UsesAliasModelForEndpoint(t *testing.T) {
	selector := NewOMLXLoadedFirstSelector(NewTestStatsCollector())
	ctx := context.WithValue(context.Background(), constants.ContextModelKey, "coder")

	coldEndpoint, coldCleanup := makeOMLXStatusEndpoint(t, "cold", map[string]bool{"Qwen-Cold": true})
	defer coldCleanup()
	loadedEndpoint, loadedCleanup := makeOMLXStatusEndpoint(t, "loaded", map[string]bool{"Qwen-Hot": true})
	defer loadedCleanup()
	storeOMLXStatus(selector, coldEndpoint, map[string]bool{"Qwen-Cold": true})
	storeOMLXStatus(selector, loadedEndpoint, map[string]bool{"Qwen-Hot": true})

	aliasMap := map[string]string{
		coldEndpoint.GetURLString():   "Qwen-Missing",
		loadedEndpoint.GetURLString(): "Qwen-Hot",
	}
	ctx = context.WithValue(ctx, constants.ContextModelAliasMapKey, aliasMap)

	selected, err := selector.Select(ctx, []*domain.Endpoint{coldEndpoint, loadedEndpoint})
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}

	if selected.Name != "loaded" {
		t.Fatalf("expected endpoint with loaded backend alias model, got %s", selected.Name)
	}
}

func TestOMLXLoadedFirstSelector_ProbeFailureFallsBackToLeastConnections(t *testing.T) {
	selector := NewOMLXLoadedFirstSelector(NewTestStatsCollector())
	ctx := context.WithValue(context.Background(), constants.ContextModelKey, "target-model")

	busyEndpoint, busyCleanup := makeOMLXStatusEndpoint(t, "busy", map[string]bool{"target-model": true})
	busyCleanup()
	idleEndpoint, idleCleanup := makeOMLXStatusEndpoint(t, "idle", map[string]bool{"target-model": false})
	defer idleCleanup()
	storeOMLXStatus(selector, idleEndpoint, map[string]bool{"target-model": false})

	selector.refreshStatus(ctx, busyEndpoint)
	selector.IncrementConnections(busyEndpoint)

	selected, err := selector.Select(ctx, []*domain.Endpoint{busyEndpoint, idleEndpoint})
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}

	if selected.Name != "idle" {
		t.Fatalf("expected least-connected fallback endpoint after probe failure, got %s", selected.Name)
	}

	if cachedStatus, ok := selector.cachedStatus(busyEndpoint.GetURLString()); !ok || len(cachedStatus.loadedModels) != 0 {
		t.Fatalf("expected failed probe to cache empty status, got ok=%t status=%v", ok, cachedStatus.loadedModels)
	}
}

func TestOMLXLoadedFirstSelector_ColdCacheDoesNotBlockSelection(t *testing.T) {
	selector := NewOMLXLoadedFirstSelector(NewTestStatsCollector())
	ctx := context.WithValue(context.Background(), constants.ContextModelKey, "target-model")

	slowEndpoint, slowCleanup := makeSlowOMLXStatusEndpoint(t, "slow", 2*defaultOMLXStatusTimeout)
	defer slowCleanup()
	idleEndpoint, idleCleanup := makeOMLXStatusEndpoint(t, "idle", map[string]bool{"target-model": false})
	defer idleCleanup()

	selector.IncrementConnections(slowEndpoint)
	started := time.Now()
	selected, err := selector.Select(ctx, []*domain.Endpoint{slowEndpoint, idleEndpoint})
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}
	elapsed := time.Since(started)

	if selected.Name != "idle" {
		t.Fatalf("expected least-connected fallback endpoint on cold cache, got %s", selected.Name)
	}
	if elapsed >= defaultOMLXStatusTimeout {
		t.Fatalf("Select blocked on status probe for %s", elapsed)
	}
}

func storeOMLXStatus(selector *OMLXLoadedFirstSelector, endpoint *domain.Endpoint, loadedModels map[string]bool) {
	selector.storeStatus(endpoint.GetURLString(), omlxStatusCacheEntry{
		fetchedAt:    time.Now(),
		loadedModels: loadedModels,
	})
}

func makeOMLXStatusEndpoint(t *testing.T, name string, loadedModels map[string]bool) (*domain.Endpoint, func()) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != defaultOMLXStatusPath {
			http.NotFound(response, request)
			return
		}

		response.Header().Set("Content-Type", "application/json")
		fmt.Fprint(response, `{"models":[`)
		first := true
		for modelID, loaded := range loadedModels {
			if !first {
				fmt.Fprint(response, ",")
			}
			first = false
			fmt.Fprintf(response, `{"id":%q,"loaded":%t}`, modelID, loaded)
		}
		fmt.Fprint(response, `]}`)
	}))

	parsedURL, err := url.Parse(server.URL)
	if err != nil {
		server.Close()
		t.Fatalf("failed to parse test server URL: %v", err)
	}

	endpoint := &domain.Endpoint{
		Name:      name,
		URL:       parsedURL,
		URLString: server.URL,
		Status:    domain.StatusHealthy,
	}

	return endpoint, server.Close
}

func makeSlowOMLXStatusEndpoint(t *testing.T, name string, delay time.Duration) (*domain.Endpoint, func()) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		time.Sleep(delay)
		response.Header().Set("Content-Type", "application/json")
		fmt.Fprint(response, `{"models":[{"id":"target-model","loaded":true}]}`)
	}))

	parsedURL, err := url.Parse(server.URL)
	if err != nil {
		server.Close()
		t.Fatalf("failed to parse test server URL: %v", err)
	}

	endpoint := &domain.Endpoint{
		Name:      name,
		URL:       parsedURL,
		URLString: server.URL,
		Status:    domain.StatusHealthy,
	}

	return endpoint, server.Close
}
