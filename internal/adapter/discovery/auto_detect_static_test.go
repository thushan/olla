package discovery

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

// These tests build the endpoint through LoadFromConfig, as production does, because the bug
// lived in how the repository resolved ModelURLString for auto endpoints. Hand-built
// endpoints (as in http_client_test.go) bypassed it.

const ollamaTagsBody = `{"models":[{"name":"llama3:8b","model":"llama3:8b","size":1},{"name":"qwen:7b","model":"qwen:7b","size":2}]}`

func openAIBody(owner string) string {
	return fmt.Sprintf(`{"object":"list","data":[{"id":"m1","object":"model","owned_by":%q},{"id":"m2","object":"model","owned_by":%q}]}`, owner, owner)
}

// discoverStatic loads cfg through the real repository and runs discovery, returning the
// model count and the profile auto-detection logged as the winner.
func discoverStatic(t *testing.T, cfg config.EndpointConfig) (int, string, error) {
	t.Helper()

	factory, err := profile.NewFactory("../../../config/profiles")
	if err != nil {
		t.Fatal(err)
	}
	repo := NewStaticEndpointRepositoryWithFactory(factory)
	if err := repo.LoadFromConfig(context.Background(), []config.EndpointConfig{cfg}); err != nil {
		t.Fatal(err)
	}
	eps, _ := repo.GetAll(context.Background())

	var logs bytes.Buffer
	lg := logger.NewPlainStyledLogger(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo})))
	c := NewHTTPModelDiscoveryClientWithDefaults(factory, lg)
	defer c.Close()

	models, derr := c.DiscoverModels(context.Background(), eps[0])
	won := ""
	for _, p := range []string{
		domain.ProfileOllama, domain.ProfileLlamaCpp, domain.ProfileLmStudio, domain.ProfileVLLM,
		domain.ProfileRapidMLX, domain.ProfileOMLX, domain.ProfileOpenAICompatible,
	} {
		if bytes.Contains(logs.Bytes(), []byte("profile="+p+"\n")) || bytes.Contains(logs.Bytes(), []byte("profile="+p+" ")) {
			won = p
		}
	}
	return len(models), won, derr
}

func TestStaticAutoDetect_WinningProfileAndModels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    string
	}{
		{
			name: "ollama api/tags only",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/tags" {
					fmt.Fprint(w, ollamaTagsBody)
					return
				}
				http.NotFound(w, r)
			},
			want: domain.ProfileOllama,
		},
		{name: "rapid-mlx", handler: openAIHandler("rapid-mlx"), want: domain.ProfileRapidMLX},
		{name: "vllm", handler: openAIHandler("vllm"), want: domain.ProfileVLLM},
		{name: "llamacpp", handler: openAIHandler("llamacpp"), want: domain.ProfileLlamaCpp},
		{name: "omlx", handler: openAIHandler("omlx"), want: domain.ProfileOMLX},
		{name: "generic openai", handler: openAIHandler("organization-owner"), want: domain.ProfileOpenAICompatible},
		{
			// An Ollama /api/tags probe must not accept an OpenAI envelope served on the same path.
			name: "unrelated envelope on api/tags is not ollama",
			handler: func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/tags":
					fmt.Fprint(w, `{"status":"ok"}`)
				case "/v1/models":
					fmt.Fprint(w, openAIBody("organization-owner"))
				default:
					http.NotFound(w, r)
				}
			},
			want: domain.ProfileOpenAICompatible,
		},
	}

	for _, typ := range []string{"auto", ""} {
		for _, tt := range tests {
			t.Run(fmt.Sprintf("type=%q/%s", typ, tt.name), func(t *testing.T) {
				t.Parallel()
				srv := httptest.NewServer(tt.handler)
				defer srv.Close()

				n, won, err := discoverStatic(t, config.EndpointConfig{Name: "e", URL: srv.URL, Type: typ})
				if err != nil {
					t.Fatalf("discovery failed: %v", err)
				}
				if won != tt.want {
					t.Errorf("winning profile = %q, want %q", won, tt.want)
				}
				if n == 0 {
					t.Errorf("discovered 0 models, want > 0")
				}
			})
		}
	}
}

func openAIHandler(owner string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, openAIBody(owner))
	}
}

// An explicit model_url still wins for every candidate under auto (issue #86).
func TestStaticAutoDetect_ExplicitModelURLHonoured(t *testing.T) {
	t.Parallel()

	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/custom/models" {
			fmt.Fprint(w, ollamaTagsBody)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	n, won, err := discoverStatic(t, config.EndpointConfig{Name: "e", URL: srv.URL, Type: "auto", ModelURL: "/custom/models"})
	if err != nil {
		t.Fatalf("discovery failed: %v", err)
	}
	if won != domain.ProfileOllama || n != 2 {
		t.Errorf("got profile %q with %d models, want ollama with 2", won, n)
	}
	for _, p := range paths {
		if p != "/custom/models" {
			t.Errorf("probed %q, want only the explicit /custom/models", p)
		}
	}
}

// Explicit types keep resolving their own profile path exactly as before.
func TestStaticExplicitType_Unchanged(t *testing.T) {
	t.Parallel()

	tests := []struct {
		typ  string
		path string
		body string
	}{
		{"ollama", "/api/tags", ollamaTagsBody},
		{"llamacpp", "/v1/models", openAIBody("llamacpp")},
		{"lm-studio", "/api/v0/models", openAIBody("lmstudio")},
	}
	for _, tt := range tests {
		t.Run(tt.typ, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == tt.path {
					fmt.Fprint(w, tt.body)
					return
				}
				http.NotFound(w, r)
			}))
			defer srv.Close()

			n, _, err := discoverStatic(t, config.EndpointConfig{Name: "e", URL: srv.URL, Type: tt.typ})
			if err != nil || n != 2 {
				t.Errorf("models=%d err=%v, want 2 models", n, err)
			}
		})
	}
}
