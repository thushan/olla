package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/logger"
)

// fcModelSpec mirrors the Flight Controller ModelSpec fields we care about for
// building Olla endpoint configs. Only Name and Port are required for routing.
type fcModelSpec struct {
	Name      string `json:"name"`
	Port      int    `json:"port"`
	Framework string `json:"framework"`
}

// fcRegistryEntry mirrors the Flight Controller RegistryEntry type returned by
// GET /registry. We only parse the fields Olla needs; extra fields are ignored.
type fcRegistryEntry struct {
	Host   string        `json:"host"`
	Models []fcModelSpec `json:"models"`
}

// FCDiscoveryPoller polls the Flight Controller /registry endpoint and reconciles
// the StaticEndpointRepository on each poll. A full replace-on-poll strategy ensures
// that endpoints removed from the FC registry are promptly evicted from Olla's rotation,
// meeting the <30s convergence acceptance criterion for petersimmons1972/instinct#12.
type FCDiscoveryPoller struct {
	repo       *StaticEndpointRepository
	registryURL string
	logger     logger.StyledLogger
	client     *http.Client
}

// NewFCDiscoveryPoller creates a poller that syncs Olla endpoints from FC /registry.
// registryBaseURL is the FC service base URL, e.g. http://ai-fleet-controller.ai-fleet.svc.cluster.local.
func NewFCDiscoveryPoller(repo *StaticEndpointRepository, registryBaseURL string, log logger.StyledLogger) *FCDiscoveryPoller {
	return &FCDiscoveryPoller{
		repo:        repo,
		registryURL: registryBaseURL + "/registry",
		logger:      log,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Poll fetches the FC registry and reconciles Olla's endpoint set.
// On FC unreachable: logs a warning, preserves the current endpoint set (fail-open).
// On success: fully replaces the endpoint set with the FC-derived list.
func (p *FCDiscoveryPoller) Poll(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.registryURL, nil)
	if err != nil {
		return fmt.Errorf("fc-discovery: build request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		// Fail-open: preserve the existing endpoint set when FC is unreachable.
		p.logger.Warn("fc-discovery: FC registry unreachable, preserving existing endpoints", "url", p.registryURL, "error", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		p.logger.Warn("fc-discovery: FC registry returned non-200, preserving existing endpoints",
			"url", p.registryURL, "status", resp.StatusCode)
		return nil
	}

	var entries []fcRegistryEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return fmt.Errorf("fc-discovery: decode registry response: %w", err)
	}

	configs := fcEntriesToEndpointConfigs(entries)
	if err := p.repo.LoadFromConfig(ctx, configs); err != nil {
		return fmt.Errorf("fc-discovery: load endpoint configs: %w", err)
	}

	p.logger.Info("fc-discovery: endpoint set reconciled from FC registry",
		"endpoints", len(configs))
	return nil
}

// RunLoop starts a background polling loop. It polls immediately on first call, then
// at the configured interval. The loop exits when ctx is cancelled.
func (p *FCDiscoveryPoller) RunLoop(ctx context.Context, interval time.Duration) {
	// Immediate first poll so Olla is populated before the ticker fires.
	if err := p.Poll(ctx); err != nil {
		p.logger.Warn("fc-discovery: initial poll error", "error", err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("fc-discovery: loop stopped")
			return
		case <-ticker.C:
			if err := p.Poll(ctx); err != nil {
				p.logger.Warn("fc-discovery: poll error", "error", err)
			}
		}
	}
}

// fcEntriesToEndpointConfigs converts FC registry entries into Olla EndpointConfig slices.
// Each (host, model) pair becomes one endpoint at http://<host>:<port>.
// Type is always "openai" because all FC-managed containers serve the OpenAI-compatible API.
// Priority defaults to 100 (standard Olla default).
func fcEntriesToEndpointConfigs(entries []fcRegistryEntry) []config.EndpointConfig {
	defaultPriority := 100
	var configs []config.EndpointConfig

	for _, entry := range entries {
		for _, model := range entry.Models {
			if model.Port == 0 {
				continue // skip models without a port (incomplete CRD)
			}
			cfg := config.EndpointConfig{
				URL:      fmt.Sprintf("http://%s:%d", entry.Host, model.Port),
				Name:     fmt.Sprintf("%s-%s", entry.Host, model.Name),
				Type:     "openai",
				Priority: &defaultPriority,
			}
			configs = append(configs, cfg)
		}
	}
	return configs
}
