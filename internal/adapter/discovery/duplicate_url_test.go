package discovery

import (
	"bytes"
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/logger"
)

// newRealProfileRepo uses the shipped profiles: the default factory resolves ./config/profiles
// against the package directory, finds nothing and falls back to a three-type built-in set.
func newRealProfileRepo(t *testing.T) *StaticEndpointRepository {
	t.Helper()
	factory, err := profile.NewFactory(filepath.Join("..", "..", "..", "config", "profiles"))
	if err != nil {
		t.Fatalf("profiles: %v", err)
	}
	return NewStaticEndpointRepositoryWithFactory(factory)
}

func TestLoadFromConfig_DuplicateURLWarns(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	repo := newRealProfileRepo(t)
	repo.SetLogger(logger.NewPlainStyledLogger(slog.New(slog.NewTextHandler(&buf, nil))))

	cfgs := []config.EndpointConfig{
		{Name: "first", URL: "http://127.0.0.1:9300", Type: "vllm"},
		{Name: "second", URL: "http://127.0.0.1:9300", Type: "sglang"},
		{Name: "other", URL: "http://127.0.0.1:9301", Type: "vllm"},
	}
	if err := repo.LoadFromConfig(context.Background(), cfgs); err != nil {
		t.Fatalf("duplicate URLs must still load: %v", err)
	}

	out := buf.String()
	for _, want := range []string{"level=WARN", "first", "second", "http://127.0.0.1:9300"} {
		if !strings.Contains(out, want) {
			t.Errorf("warning missing %q: %s", want, out)
		}
	}
	if strings.Contains(out, "other") {
		t.Errorf("unique endpoint named in warning: %s", out)
	}
	all, _ := repo.GetAll(context.Background())
	if len(all) != 2 {
		t.Errorf("registered %d endpoints, want 2", len(all))
	}
}

func TestDuplicateEndpointURLWarnings(t *testing.T) {
	t.Parallel()

	got := DuplicateEndpointURLWarnings([]config.EndpointConfig{
		{Name: "a", URL: "http://h:1"},
		{Name: "b", URL: "http://h:1"},
		{Name: "c", URL: "http://h:2"},
	})
	if len(got) != 1 || !strings.Contains(got[0], `"b"`) || !strings.Contains(got[0], `"a"`) {
		t.Fatalf("unexpected warnings: %v", got)
	}
	if got := DuplicateEndpointURLWarnings(nil); len(got) != 0 {
		t.Fatalf("expected none, got %v", got)
	}
}

func TestDuplicateEndpointURLWarnings_RedactsCredentials(t *testing.T) {
	t.Parallel()

	got := DuplicateEndpointURLWarnings([]config.EndpointConfig{
		{Name: "a", URL: "http://user:secret@h:1"},
		{Name: "b", URL: "http://user:secret@h:1"},
	})
	if len(got) != 1 || strings.Contains(got[0], "secret") || strings.Contains(got[0], "user") {
		t.Fatalf("warning leaked credentials or was missed: %v", got)
	}
}

// The sticky manual config once declared 24 endpoints on three URLs, so only the last
// provider group registered and seven provider routes 404ed.
func TestStickyManualConfig_AllEndpointsRegister(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load(filepath.Join("..", "..", "..", "test", "manual", "config.sticky.yaml"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	configured := cfg.Discovery.Static.Endpoints
	if len(configured) == 0 {
		t.Fatal("no endpoints configured")
	}
	repo := newRealProfileRepo(t)
	if err := repo.LoadFromConfig(context.Background(), configured); err != nil {
		t.Fatalf("LoadFromConfig: %v", err)
	}
	all, _ := repo.GetAll(context.Background())
	if len(all) != len(configured) {
		t.Errorf("registered %d of %d endpoints; URLs must be unique: %v",
			len(all), len(configured), DuplicateEndpointURLWarnings(configured))
	}
}
