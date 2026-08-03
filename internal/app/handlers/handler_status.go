package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"hash"
	"hash/fnv"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/version"

	"github.com/thushan/olla/internal/config"

	"github.com/thushan/olla/internal/util"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/pkg/format"
)

var (
	statusHealthy  = "healthy"
	statusDegraded = "degraded"
	statusCritical = "critical"
	statusNormal   = "normal"
	statusElevated = "elevated"
	zeroTraffic    = "0 B"
	emptyString    = ""
)

type SystemSummary struct {
	// Additive (FR-13): absolute process start so the dashboard can compute a
	// live uptime between polls without refetching. Not omitempty: always known
	// once the process is up. Sits alongside the existing relative uptime string.
	StartTime          time.Time `json:"start_time"`
	Status             string    `json:"status"`
	EndpointsUp        string    `json:"endpoints_up"`
	SuccessRate        string    `json:"success_rate"`
	AvgLatency         string    `json:"avg_latency"`
	TotalTraffic       string    `json:"total_traffic"`
	UptimeHuman        string    `json:"uptime"`
	Version            string    `json:"version"`
	Commit             string    `json:"commit"`
	ActiveConnections  int64     `json:"active_connections"`
	SecurityViolations int64     `json:"security_violations"`
	TotalRequests      int64     `json:"total_requests"`
	TotalFailures      int64     `json:"total_failures"`
	// Additive (FR-13): fleet-wide latency rollup in raw ms alongside the
	// formatted AvgLatency string. Plain int64 (not omitempty) so an idle
	// fleet still serialises a zero, mirroring TotalRequests and the per
	// endpoint MinLatencyMs/MaxLatencyMs convention.
	MinLatencyMs int64 `json:"min_latency_ms"`
	MaxLatencyMs int64 `json:"max_latency_ms"`
}

type ProxySummary struct {
	Engine   string `json:"engine"`
	Profile  string `json:"profile"`
	Balancer string `json:"balancer"`
}

type EndpointResponse struct {
	AvgLatencyMs  *int64                 `json:"avg_latency_ms,omitempty"`
	NextCheckAt   *time.Time             `json:"next_check_at,omitempty"`
	HealthCheckAt *time.Time             `json:"health_check_at,omitempty"`
	Models        EndpointModelsResponse `json:"models"`
	Name          string                 `json:"name"`
	Status        string                 `json:"status"`
	SuccessRate   string                 `json:"success_rate"`
	AvgLatency    string                 `json:"avg_latency"`
	Traffic       string                 `json:"traffic"`
	LastCheck     string                 `json:"last_check"`
	NextCheck     string                 `json:"next_check"`
	Issues        string                 `json:"issues"`
	URL           string                 `json:"url"`
	// ID is a stable identifier derived from the raw (unsanitised) endpoint
	// URL; see stableEndpointID in handler_status_endpoints.go.
	ID          string `json:"id"`
	Priority    int    `json:"priority"`
	Connections int64  `json:"connections"`
	Requests    int64  `json:"requests"`
	// Additive dashboard fields (FR-13: existing fields above are unchanged).
	// active_connections is intentionally NOT duplicated here: Connections
	// already carries the same value. last_model_sync_at is intentionally NOT
	// duplicated: Models.LastUpdated already serialises as an RFC3339 absolute.
	MinLatencyMs int64 `json:"min_latency_ms"`
	MaxLatencyMs int64 `json:"max_latency_ms"`
}

type EndpointModelsResponse struct {
	LastUpdated time.Time `json:"last_updated"`
	Count       int64     `json:"count"`
}

type SecuritySummary struct {
	Status     string            `json:"status"`
	BlockedIPs int               `json:"blocked_ips"`
	Violations SecurityViolation `json:"violations"`
}
type SecurityViolation struct {
	RateLimits int64 `json:"rate_limits"`
	SizeLimits int64 `json:"size_limits"`
}
type StatusResponse struct {
	Timestamp time.Time          `json:"timestamp"`
	Proxy     ProxySummary       `json:"proxy"`
	Endpoints []EndpointResponse `json:"endpoints"`
	Security  SecuritySummary    `json:"security"`
	System    SystemSummary      `json:"system"`
}

type statusSnapshot struct {
	endpointStats   map[string]ports.EndpointStats
	connectionStats map[string]int64
	endpointModels  map[string]*domain.EndpointModels
	all             []*domain.Endpoint
	healthy         []*domain.Endpoint
	proxyStats      ports.ProxyStats
	securityStats   ports.SecurityStats
}

func (a *Application) gatherStatusSnapshot(ctx context.Context) (*statusSnapshot, error) {
	all, healthy, _, err := a.getEndpointCounts(ctx)
	if err != nil {
		return nil, err
	}

	endpointModelsMap, emErr := a.modelRegistry.GetEndpointModelMap(ctx)
	if emErr != nil {
		a.logger.Warn("Failed to get model map", "error", emErr)
		endpointModelsMap = make(map[string]*domain.EndpointModels)
	}

	var endpointStats map[string]ports.EndpointStats
	var proxyStats ports.ProxyStats
	var securityStats ports.SecurityStats
	var connectionStats map[string]int64

	if a.statsCollector != nil {
		endpointStats = a.statsCollector.GetEndpointStats()
		proxyStats = a.statsCollector.GetProxyStats()
		securityStats = a.statsCollector.GetSecurityStats()
		connectionStats = a.statsCollector.GetConnectionStats()
	}

	return &statusSnapshot{
		all:             all,
		healthy:         healthy,
		endpointStats:   endpointStats,
		proxyStats:      proxyStats,
		securityStats:   securityStats,
		connectionStats: connectionStats,
		endpointModels:  endpointModelsMap,
	}, nil
}

func (a *Application) buildStatusResponse(snapshot *statusSnapshot) StatusResponse {
	response := StatusResponse{
		Timestamp: time.Now(),
		Endpoints: make([]EndpointResponse, len(snapshot.all)),
	}

	response.Proxy = a.buildProxySummary(a.Config.Proxy)
	response.System = a.buildSystemSummary(snapshot.all, snapshot.healthy, snapshot.proxyStats, snapshot.securityStats, snapshot.connectionStats, snapshot.endpointStats)
	a.buildUnifiedEndpoints(snapshot.all, snapshot.endpointStats, snapshot.connectionStats, response.Endpoints, snapshot.endpointModels)
	response.Security = a.buildSecuritySummary(snapshot.securityStats)

	return response
}

func (a *Application) statusHandler(w http.ResponseWriter, r *http.Request) {
	snapshot, err := a.gatherStatusSnapshot(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get endpoint data: %v", err), http.StatusInternalServerError)
		return
	}

	response := a.buildStatusResponse(snapshot)

	body, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
	writeJSONWithETag(w, r, body, hashStatusResponse(&response))
}

func (a *Application) buildProxySummary(proxyConfig config.ProxyConfig) ProxySummary {
	return ProxySummary{
		Engine:   proxyConfig.Engine,
		Profile:  proxyConfig.Profile,
		Balancer: proxyConfig.LoadBalancer,
	}
}

func (a *Application) buildSystemSummary(all, healthy []*domain.Endpoint, proxy ports.ProxyStats, security ports.SecurityStats, connections map[string]int64, endpointStats map[string]ports.EndpointStats) SystemSummary {
	var totalConnections, totalTraffic int64

	for url, conn := range connections {
		totalConnections += conn
		if stats, exists := endpointStats[url]; exists {
			totalTraffic += stats.TotalBytes
		}
	}

	// ratios
	healthyRatio := float64(len(healthy)) / float64(len(all))
	var systemSuccessRate float64
	if proxy.TotalRequests > 0 {
		systemSuccessRate = float64(proxy.SuccessfulRequests) / float64(proxy.TotalRequests) * 100.0
	}

	var status string
	switch {
	case healthyRatio < 0.5 || systemSuccessRate < 90.0:
		status = statusCritical
	case healthyRatio < 0.8 || systemSuccessRate < 95.0:
		status = statusDegraded
	default:
		status = statusHealthy
	}

	totalViolations := security.RateLimitViolations + security.SizeLimitViolations

	return SystemSummary{
		Version:            version.Version,
		Commit:             version.Commit,
		Status:             status,
		EndpointsUp:        format.EndpointsUp(len(healthy), len(all)),
		SuccessRate:        format.Percentage(systemSuccessRate),
		AvgLatency:         format.Latency(proxy.AverageLatency),
		ActiveConnections:  totalConnections,
		SecurityViolations: totalViolations,
		TotalTraffic:       format.Bytes(util.SafeUint64(totalTraffic)),
		TotalRequests:      proxy.TotalRequests,
		TotalFailures:      proxy.FailedRequests,
		MinLatencyMs:       proxy.MinLatency,
		MaxLatencyMs:       proxy.MaxLatency,
		UptimeHuman:        format.Duration2(time.Since(a.StartTime)),
		StartTime:          a.StartTime,
	}
}

func (a *Application) buildUnifiedEndpoints(all []*domain.Endpoint, statsMap map[string]ports.EndpointStats,
	connectionStats map[string]int64, endpoints []EndpointResponse, modelMap map[string]*domain.EndpointModels) {
	for i, endpoint := range all {
		url := endpoint.GetURLString()
		stats, hasStats := statsMap[url]
		connections := connectionStats[url]
		endpointModels := modelMap[url]

		var successRate float64
		if hasStats && stats.TotalRequests > 0 {
			successRate = float64(stats.SuccessfulRequests) / float64(stats.TotalRequests) * 100.0
		}

		traffic := zeroTraffic
		requests := int64(0)
		avgLatency := int64(0)
		if hasStats {
			traffic = format.Bytes(util.SafeUint64(stats.TotalBytes))
			requests = stats.TotalRequests
			avgLatency = stats.AverageLatency
		}

		var modelDisco EndpointModelsResponse
		if endpointModels != nil {
			modelDisco = EndpointModelsResponse{
				LastUpdated: endpointModels.LastUpdated,
				Count:       int64(len(endpointModels.Models)),
			}
		}

		endpoints[i] = EndpointResponse{
			Name:        endpoint.Name,
			Status:      endpoint.Status.String(),
			Priority:    endpoint.Priority,
			Connections: connections,
			Requests:    requests,
			SuccessRate: format.Percentage(successRate),
			AvgLatency:  format.Latency(avgLatency),
			Traffic:     traffic,
			LastCheck:   format.TimeAgo(endpoint.LastChecked),
			NextCheck:   format.TimeUntil(endpoint.NextCheckTime),
			Models:      modelDisco,
			Issues:      a.getEndpointIssues(endpoint, stats, hasStats, successRate),
			ID:          stableEndpointID(url),
			URL:         sanitiseDisplayURL(url),
		}

		// Additive absolute timestamps alongside the existing relative strings.
		if !endpoint.LastChecked.IsZero() {
			lc := endpoint.LastChecked
			endpoints[i].HealthCheckAt = &lc
		}
		if !endpoint.NextCheckTime.IsZero() {
			nc := endpoint.NextCheckTime
			endpoints[i].NextCheckAt = &nc
		}

		// Raw latency in ms alongside the formatted AvgLatency string. Only
		// meaningful under traffic; min/max match the plain-zero convention of
		// Requests, avg is a pointer so a no-traffic endpoint omits the field.
		if hasStats && stats.TotalRequests > 0 {
			endpoints[i].MinLatencyMs = stats.MinLatency
			endpoints[i].MaxLatencyMs = stats.MaxLatency
			avgMs := stats.AverageLatency
			endpoints[i].AvgLatencyMs = &avgMs
		}
	}

	sort.Slice(endpoints, func(i, j int) bool {
		if endpoints[i].Priority != endpoints[j].Priority {
			return endpoints[i].Priority > endpoints[j].Priority
		}
		// Within a priority band, healthy comes before anything else.
		iHealthy := endpoints[i].Status == statusHealthy
		jHealthy := endpoints[j].Status == statusHealthy
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
		if endpoints[i].Name != endpoints[j].Name {
			return endpoints[i].Name < endpoints[j].Name
		}
		return endpoints[i].ID < endpoints[j].ID
	})
}

func (a *Application) buildSecuritySummary(stats ports.SecurityStats) SecuritySummary {
	var status string
	totalViolations := stats.RateLimitViolations + stats.SizeLimitViolations

	if totalViolations > 100 || stats.UniqueRateLimitedIPs > 10 {
		status = statusElevated
	} else {
		status = statusNormal
	}

	return SecuritySummary{
		Violations: SecurityViolation{
			RateLimits: stats.RateLimitViolations,
			SizeLimits: stats.SizeLimitViolations,
		},
		BlockedIPs: stats.UniqueRateLimitedIPs,
		Status:     status,
	}
}

func (a *Application) getEndpointIssues(endpoint *domain.Endpoint, stats ports.EndpointStats, hasStats bool, successRate float64) string {
	issues := make([]string, 0, 4)

	if endpoint.ConsecutiveFailures > 3 {
		issues = append(issues, "consecutive failures")
	}

	if hasStats {
		if successRate < 90.0 && stats.TotalRequests > 10 {
			issues = append(issues, "low success rate")
		}
		if stats.AverageLatency > 5000 {
			issues = append(issues, "high latency")
		}
	}

	if endpoint.Status == domain.StatusOffline || endpoint.Status == domain.StatusUnhealthy {
		issues = append(issues, "unavailable")
	}

	if len(issues) == 0 {
		return emptyString
	}

	return strings.Join(issues, ", ")
}

func (a *Application) getEndpointCounts(ctx context.Context) (all, healthy, routable []*domain.Endpoint, err error) {
	if all, err = a.repository.GetAll(ctx); err != nil {
		return
	}
	if healthy, err = a.repository.GetHealthy(ctx); err != nil {
		return
	}
	if routable, err = a.repository.GetRoutable(ctx); err != nil {
		return
	}
	return
}

// etagSep terminates each hashed field so adjacent fields cannot collide:
// without it ("ab"+"c") and ("a"+"bc") would feed the hash identical bytes.
var etagSep = []byte{0}

// hashEtagString writes a string field to the ETag hasher. Writes never error
// for fnv's hasher, the discarded error keeps the hash.Hash contract visible.
func hashEtagString(h hash.Hash, s string) {
	_, _ = h.Write([]byte(s))
	_, _ = h.Write(etagSep)
}

// hashEtagInt64 writes an integer field in base 10. Locale-independent so the
// ETag stays stable regardless of runtime environment.
func hashEtagInt64(h hash.Hash, v int64) {
	var buf [20]byte
	_, _ = h.Write(strconv.AppendInt(buf[:0], v, 10))
	_, _ = h.Write(etagSep)
}

// hashEtagInt64Ptr encodes presence: a nil pointer emits a distinct field
// boundary from a zero, mirroring omitempty semantics in the wire payload.
func hashEtagInt64Ptr(h hash.Hash, v *int64) {
	if v == nil {
		_, _ = h.Write(etagSep)
		return
	}
	hashEtagInt64(h, *v)
}

// hashEtagTime encodes a wall-clock instant via UnixNano. Only absolute event
// times reach this path; the relative time-ago / time-until renderings used in
// the dashboard (uptime, last_check, next_check, last_seen, last_model_sync)
// are deliberately excluded from the hash because their one-second granularity
// would change on every poll and defeat the ETag.
func hashEtagTime(h hash.Hash, t time.Time) {
	if t.IsZero() {
		_, _ = h.Write(etagSep)
		return
	}
	hashEtagInt64(h, t.UnixNano())
}

func hashEtagTimePtr(h hash.Hash, t *time.Time) {
	if t == nil {
		_, _ = h.Write(etagSep)
		return
	}
	hashEtagTime(h, *t)
}

// hashEtagStringSlice encodes a slice in order. Callers must guarantee the
// slice is already sorted deterministically; the dashboard's model/endpoint
// slices all sort before serialisation for diffable output across polls.
func hashEtagStringSlice(h hash.Hash, s []string) {
	for _, x := range s {
		hashEtagString(h, x)
	}
	_, _ = h.Write(etagSep)
}

// formatEtag renders the FNV-1a sum as a quoted strong validator. Base36
// mirrors the style of stableEndpointID so client and server identity formats
// read the same.
func formatEtag(h hash.Hash32) string {
	return `"` + strconv.FormatUint(uint64(h.Sum32()), 36) + `"`
}

// etagMatches implements the If-None-Match comparison: a bare "*" matches any
// current representation, otherwise each listed validator is compared for
// exact equality against the response ETag. Weak validators (W/"...") are not
// emitted by these handlers, so strong-vs-weak comparison rules do not apply.
func etagMatches(ifNoneMatch, etag string) bool {
	if ifNoneMatch == "" {
		return false
	}
	if ifNoneMatch == "*" {
		return true
	}
	for _, t := range strings.Split(ifNoneMatch, ",") {
		if strings.TrimSpace(t) == etag {
			return true
		}
	}
	return false
}

// writeJSONWithETag emits a buffered JSON body with a strong-validator ETag,
// returning a bare 304 (no body, no Content-Encoding) when the client's
// If-None-Match matches. Headers are set before WriteHeader so they reach the
// client on both 200 and 304 paths.
func writeJSONWithETag(w http.ResponseWriter, r *http.Request, body []byte, etag string) {
	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	w.Header().Set("ETag", etag)
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// hashStatusResponse computes the FNV-1a ETag over the stable fields of a
// StatusResponse, deliberately excluding the top-level Timestamp and every
// relative time-ago string. Absolute event times (start_time, health_check_at,
// next_check_at, models.last_updated) are stable real data and stay in.
func hashStatusResponse(resp *StatusResponse) string {
	h := fnv.New32a()
	hashEtagString(h, resp.Proxy.Engine)
	hashEtagString(h, resp.Proxy.Profile)
	hashEtagString(h, resp.Proxy.Balancer)

	hashEtagTime(h, resp.System.StartTime)
	hashEtagString(h, resp.System.Status)
	hashEtagString(h, resp.System.EndpointsUp)
	hashEtagString(h, resp.System.SuccessRate)
	hashEtagString(h, resp.System.AvgLatency)
	hashEtagString(h, resp.System.TotalTraffic)
	hashEtagString(h, resp.System.Version)
	hashEtagString(h, resp.System.Commit)
	hashEtagInt64(h, resp.System.ActiveConnections)
	hashEtagInt64(h, resp.System.SecurityViolations)
	hashEtagInt64(h, resp.System.TotalRequests)
	hashEtagInt64(h, resp.System.TotalFailures)
	hashEtagInt64(h, resp.System.MinLatencyMs)
	hashEtagInt64(h, resp.System.MaxLatencyMs)

	hashEtagString(h, resp.Security.Status)
	hashEtagInt64(h, int64(resp.Security.BlockedIPs))
	hashEtagInt64(h, resp.Security.Violations.RateLimits)
	hashEtagInt64(h, resp.Security.Violations.SizeLimits)

	for i := range resp.Endpoints {
		hashEndpointResponse(h, &resp.Endpoints[i])
	}
	return formatEtag(h)
}

// hashEndpointResponse feeds the stable fields of one EndpointResponse to the
// hasher. Relative strings (last_check, next_check) are skipped because they
// render with one-second granularity and would change on every poll.
func hashEndpointResponse(h hash.Hash, ep *EndpointResponse) {
	hashEtagString(h, ep.ID)
	hashEtagString(h, ep.Name)
	hashEtagString(h, ep.Status)
	hashEtagString(h, ep.URL)
	hashEtagString(h, ep.SuccessRate)
	hashEtagString(h, ep.AvgLatency)
	hashEtagString(h, ep.Traffic)
	hashEtagString(h, ep.Issues)
	hashEtagInt64(h, int64(ep.Priority))
	hashEtagInt64(h, ep.Connections)
	hashEtagInt64(h, ep.Requests)
	hashEtagInt64(h, ep.MinLatencyMs)
	hashEtagInt64(h, ep.MaxLatencyMs)
	hashEtagInt64Ptr(h, ep.AvgLatencyMs)
	hashEtagTimePtr(h, ep.NextCheckAt)
	hashEtagTimePtr(h, ep.HealthCheckAt)
	hashEtagTime(h, ep.Models.LastUpdated)
	hashEtagInt64(h, ep.Models.Count)
}
