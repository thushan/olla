package handlers

import (
	"encoding/json"
	"hash"
	"hash/fnv"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/thushan/olla/internal/core/ports"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/pkg/format"
)

type EndpointSummary struct {
	AvgLatencyMs    *int64     `json:"avg_latency_ms,omitempty"`
	NextCheckAt     *time.Time `json:"next_check_at,omitempty"`
	HealthCheckAt   *time.Time `json:"health_check_at,omitempty"`
	LastModelSyncAt *time.Time `json:"last_model_sync_at,omitempty"`
	Name            string     `json:"name"`
	Type            string     `json:"type"`
	Status          string     `json:"status"`
	LastModelSync   string     `json:"last_model_sync,omitempty"`
	HealthCheck     string     `json:"health_check"`
	ResponseTime    string     `json:"response_time,omitempty"`
	SuccessRate     string     `json:"success_rate"`
	Issues          string     `json:"issues,omitempty"`
	URL             string     `json:"url"`
	// ID is a stable identifier derived from the raw (unsanitised) endpoint
	// URL, added because URL above has query/fragment stripped for display
	// and can therefore collide across distinct endpoints. See
	// stableEndpointID for the derivation and its collision caveat.
	ID           string `json:"id"`
	Priority     int    `json:"priority"`
	ModelCount   int    `json:"model_count"`
	RequestCount int64  `json:"request_count"`
	// Additive dashboard fields, existing fields above are unchanged.
	// min/max latency follow the same plain-zero convention as RequestCount
	// for no-traffic endpoints; avg_latency_ms is a pointer so a no-traffic
	// endpoint omits the field rather than emitting a misleading 0.
	MinLatencyMs      int64 `json:"min_latency_ms"`
	MaxLatencyMs      int64 `json:"max_latency_ms"`
	ActiveConnections int64 `json:"active_connections"`
}

type EndpointStatusResponse struct {
	Timestamp     time.Time         `json:"timestamp"`
	Endpoints     []EndpointSummary `json:"endpoints"`
	TotalCount    int               `json:"total_count"`
	HealthyCount  int               `json:"healthy_count"`
	RoutableCount int               `json:"routable_count"`
}

const (
	healthyStatus = "healthy"
)

func (a *Application) endpointsStatusHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// get everyone all at once, then deal with it ted.
	allEndpoints, healthyEndpoints, routableEndpoints, err := a.getEndpointCounts(ctx)
	if err != nil {
		http.Error(w, "Failed to get endpoint data", http.StatusInternalServerError)
		return
	}

	endpointStats := a.statsCollector.GetEndpointStats()
	connectionStats := a.statsCollector.GetConnectionStats()
	modelMap, _ := a.modelRegistry.GetEndpointModelMap(ctx)
	summaries := make([]EndpointSummary, 0, len(allEndpoints))

	for _, endpoint := range allEndpoints {
		summary := a.buildEndpointSummaryOptimised(endpoint, endpointStats, connectionStats, modelMap)
		summaries = append(summaries, summary)
	}

	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].Priority != summaries[j].Priority {
			return summaries[i].Priority > summaries[j].Priority
		}
		// Within a priority band, healthy comes before anything else.
		iHealthy := summaries[i].Status == healthyStatus
		jHealthy := summaries[j].Status == healthyStatus
		if iHealthy != jHealthy {
			return iHealthy
		}
		// Tie-breaker for deterministic ordering across polls: the input
		// slice comes from map iteration, so without a final comparison
		// equal-priority same-health endpoints reorder between polls purely
		// from map-iteration randomisation. Name first, then ID for the
		// pathological case of two endpoints sharing a name. ID (not the
		// sanitised URL) because sanitisation strips query/fragment and two
		// distinct endpoints can share a display URL, which would leave the
		// comparison tied and ordering unstable again.
		if summaries[i].Name != summaries[j].Name {
			return summaries[i].Name < summaries[j].Name
		}
		return summaries[i].ID < summaries[j].ID
	})

	// create a response with minimal mallocs
	response := EndpointStatusResponse{
		Timestamp:     time.Now(),
		TotalCount:    len(allEndpoints),
		HealthyCount:  len(healthyEndpoints),
		RoutableCount: len(routableEndpoints),
		Endpoints:     summaries,
	}

	body, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
	writeJSONWithETag(w, r, body, hashEndpointStatusResponse(&response))
}

func (a *Application) buildEndpointSummaryOptimised(endpoint *domain.Endpoint, statsMap map[string]ports.EndpointStats, connectionStats map[string]int64, modelMap map[string]*domain.EndpointModels) EndpointSummary {
	url := endpoint.URLString
	stats, hasStats := statsMap[url]
	models := modelMap[url]

	summary := EndpointSummary{
		Name:              endpoint.Name,
		Type:              endpoint.Type,
		Status:            endpoint.Status.String(),
		Priority:          endpoint.Priority,
		ID:                stableEndpointID(url),
		URL:               sanitiseDisplayURL(url),
		ActiveConnections: connectionStats[url],
	}

	if models != nil {
		summary.ModelCount = len(models.Models)
		if !models.LastUpdated.IsZero() {
			summary.LastModelSync = format.TimeAgo(models.LastUpdated)
			latm := models.LastUpdated
			summary.LastModelSyncAt = &latm
		}
	}

	if !endpoint.LastChecked.IsZero() {
		summary.HealthCheck = format.TimeAgo(endpoint.LastChecked)
		lc := endpoint.LastChecked
		summary.HealthCheckAt = &lc
		if endpoint.LastLatency > 0 {
			summary.ResponseTime = format.Latency(endpoint.LastLatency.Milliseconds())
		}
	}

	if !endpoint.NextCheckTime.IsZero() {
		nc := endpoint.NextCheckTime
		summary.NextCheckAt = &nc
	}

	if hasStats {
		summary.RequestCount = stats.TotalRequests
		if stats.TotalRequests > 0 {
			successRate := (float64(stats.SuccessfulRequests) * 100.0) / float64(stats.TotalRequests)
			summary.SuccessRate = format.Percentage(successRate)
			// Min/max/avg are only meaningful with traffic; min/max follow the
			// plain-zero convention of RequestCount, avg is a pointer so it is
			// omitted entirely when there is no traffic.
			summary.MinLatencyMs = stats.MinLatency
			summary.MaxLatencyMs = stats.MaxLatency
			avg := stats.AverageLatency
			summary.AvgLatencyMs = &avg
		} else {
			summary.SuccessRate = "N/A"
		}
	} else {
		summary.SuccessRate = "N/A"
	}

	summary.Issues = a.getEndpointIssuesSummaryOptimised(endpoint, stats, hasStats)

	return summary
}

func (a *Application) getEndpointIssuesSummaryOptimised(endpoint *domain.Endpoint, stats ports.EndpointStats, hasStats bool) string {
	if endpoint.Status == domain.StatusHealthy && endpoint.ConsecutiveFailures == 0 {
		return ""
	}

	if endpoint.Status == domain.StatusOffline || endpoint.Status == domain.StatusUnhealthy {
		return "unavailable"
	}

	if endpoint.ConsecutiveFailures > 3 {
		return "unstable"
	}

	if hasStats && stats.TotalRequests > 10 {
		if stats.SuccessfulRequests*100 < stats.TotalRequests*90 {
			return "low success rate"
		}
	}

	return ""
}

// stableEndpointID derives a stable, opaque identifier from the raw
// (unsanitised) endpoint URL, shared by both the /internal/status and
// /internal/status/endpoints handlers. It exists because sanitiseDisplayURL
// strips RawQuery and Fragment before a URL reaches the frontend, so two
// distinct endpoints (e.g. differing only by a query string) can render with
// an identical display URL. The repository keys its endpoint map on the raw
// URL string including query and fragment, so those two endpoints really are
// distinct - the frontend needs an identity that survives sanitisation to
// key rows on and to break sort ties deterministically.
//
// FNV-1a 32-bit rendered as base36 mirrors the stableId() hash the dashboard
// already uses client-side for DOM ids (web/dashboard/src/lib/dom-id.js), so
// both layers derive identity the same way. A 32-bit hash is not injective -
// two distinct URLs can theoretically collide - but that is an acceptable,
// collision-resistant trade-off at realistic fleet sizes (tens to low
// hundreds of endpoints), not a correctness requirement.
func stableEndpointID(raw string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(raw))
	return strconv.FormatUint(uint64(h.Sum32()), 36)
}

// hashEndpointStatusResponse computes the FNV-1a ETag for the
// /internal/status/endpoints payload, excluding the top-level Timestamp and
// the relative time-ago renderings (health_check, last_model_sync) that change
// every poll. Absolute event times stay in.
func hashEndpointStatusResponse(resp *EndpointStatusResponse) string {
	h := fnv.New32a()
	for i := range resp.Endpoints {
		hashEndpointSummary(h, &resp.Endpoints[i])
	}
	hashEtagInt64(h, int64(resp.TotalCount))
	hashEtagInt64(h, int64(resp.HealthyCount))
	hashEtagInt64(h, int64(resp.RoutableCount))
	return formatEtag(h)
}

// hashEndpointSummary feeds the stable fields of one EndpointSummary. Relative
// strings (HealthCheck, LastModelSync) are skipped; ResponseTime is kept as it
// renders from endpoint.LastLatency, an absolute measurement, not the wall clock.
func hashEndpointSummary(h hash.Hash, s *EndpointSummary) {
	hashEtagString(h, s.ID)
	hashEtagString(h, s.Name)
	hashEtagString(h, s.Type)
	hashEtagString(h, s.Status)
	hashEtagString(h, s.URL)
	hashEtagString(h, s.SuccessRate)
	hashEtagString(h, s.ResponseTime)
	hashEtagString(h, s.Issues)
	hashEtagInt64(h, int64(s.Priority))
	hashEtagInt64(h, int64(s.ModelCount))
	hashEtagInt64(h, s.RequestCount)
	hashEtagInt64(h, s.MinLatencyMs)
	hashEtagInt64(h, s.MaxLatencyMs)
	hashEtagInt64(h, s.ActiveConnections)
	hashEtagInt64Ptr(h, s.AvgLatencyMs)
	hashEtagTimePtr(h, s.NextCheckAt)
	hashEtagTimePtr(h, s.HealthCheckAt)
	hashEtagTimePtr(h, s.LastModelSyncAt)
}

// unparseableURLSentinel is returned by sanitiseDisplayURL when url.Parse
// rejects the input. It must never be the raw string: a parse failure can
// occur on a credentialed URL (e.g. a space inside the password), and
// returning the input verbatim would surface those credentials in the
// status/dashboard JSON. The sentinel is a value an operator can still
// recognise as "this endpoint's URL is malformed" without exposing what was
// inside it. Config-load validation only catches URLs that parse, so an
// unparseable credentialed URL is reachable here.
const unparseableURLSentinel = "[unparseable url]"

// sanitiseDisplayURL strips userinfo, query and fragment from an endpoint URL
// before it is surfaced in any status/dashboard JSON. Credentials must
// never appear in a URL string in responses; the endorsed credential path is
// the auth config block, which is held as json:"-" fields on domain.Endpoint
// and never reaches this layer. RawQuery and Fragment are stripped wholesale
// rather than allowlisting "safe" params: query strings on inference backends
// are not part of the operator-facing identity of an endpoint and a per-key
// allowlist is a maintenance trap. On any parse error the sentinel above is
// returned so the operator sees a diagnosable placeholder, never the raw
// input: a credential-bearing URL that fails to parse must not leak.
func sanitiseDisplayURL(raw string) string {
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return unparseableURLSentinel
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.RawFragment = ""
	return parsed.String()
}
