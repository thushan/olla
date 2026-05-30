package balancer

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

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