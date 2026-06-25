package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/version"
)

func (a *Application) metricsHandler(w http.ResponseWriter, r *http.Request) {
	snapshot, err := a.gatherStatusSnapshot(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get endpoint data: %v", err), http.StatusInternalServerError)
		return
	}

	response := a.buildStatusResponse(snapshot)

	w.Header().Set(constants.HeaderContentType, constants.ContentTypePrometheus)
	w.WriteHeader(http.StatusOK)
	writePrometheusMetrics(w, response, snapshot, a.Config.Proxy, time.Since(a.StartTime))
}

func writePrometheusMetrics(w http.ResponseWriter, response StatusResponse, snapshot *statusSnapshot, proxyConfig config.ProxyConfig, uptime time.Duration) {
	var b strings.Builder

	writePrometheusHelpType(&b, "olla_info", "gauge", "Olla build and proxy configuration")
	writePrometheusLabeledGauge(&b, "olla_info", 1,
		"version", version.Version,
		"commit", version.Commit,
		"engine", proxyConfig.Engine,
		"profile", proxyConfig.Profile,
		"balancer", proxyConfig.LoadBalancer,
	)

	writePrometheusHelpType(&b, "olla_system_status", "gauge", "Overall system status (2=healthy, 1=degraded, 0=critical)")
	writePrometheusGauge(&b, "olla_system_status", systemStatusValue(response.System.Status))

	writePrometheusHelpType(&b, "olla_endpoints_total", "gauge", "Total configured endpoints")
	writePrometheusGauge(&b, "olla_endpoints_total", float64(len(snapshot.all)))

	writePrometheusHelpType(&b, "olla_endpoints_healthy", "gauge", "Number of healthy endpoints")
	writePrometheusGauge(&b, "olla_endpoints_healthy", float64(len(snapshot.healthy)))

	writePrometheusHelpType(&b, "olla_success_rate_percent", "gauge", "Proxy success rate percentage")
	writePrometheusGauge(&b, "olla_success_rate_percent", parsePercentage(response.System.SuccessRate))

	writePrometheusHelpType(&b, "olla_avg_latency_ms", "gauge", "Average proxy latency in milliseconds")
	writePrometheusGauge(&b, "olla_avg_latency_ms", float64(snapshot.proxyStats.AverageLatency))

	writePrometheusHelpType(&b, "olla_total_traffic_bytes", "gauge", "Total traffic across all endpoints in bytes")
	writePrometheusGauge(&b, "olla_total_traffic_bytes", float64(totalTrafficBytes(snapshot)))

	writePrometheusHelpType(&b, "olla_uptime_seconds", "gauge", "Olla process uptime in seconds")
	writePrometheusGauge(&b, "olla_uptime_seconds", uptime.Seconds())

	writePrometheusHelpType(&b, "olla_active_connections", "gauge", "Active connections across all endpoints")
	writePrometheusGauge(&b, "olla_active_connections", float64(response.System.ActiveConnections))

	writePrometheusHelpType(&b, "olla_security_violations_total", "counter", "Total security violations")
	writePrometheusGauge(&b, "olla_security_violations_total", float64(response.System.SecurityViolations))

	writePrometheusHelpType(&b, "olla_requests_total", "counter", "Total proxy requests processed")
	writePrometheusGauge(&b, "olla_requests_total", float64(response.System.TotalRequests))

	writePrometheusHelpType(&b, "olla_failures_total", "counter", "Total failed proxy requests")
	writePrometheusGauge(&b, "olla_failures_total", float64(response.System.TotalFailures))

	writePrometheusHelpType(&b, "olla_security_blocked_ips", "gauge", "Unique IPs blocked by rate limiting")
	writePrometheusGauge(&b, "olla_security_blocked_ips", float64(response.Security.BlockedIPs))

	writePrometheusHelpType(&b, "olla_security_rate_limit_violations_total", "counter", "Rate limit violations")
	writePrometheusGauge(&b, "olla_security_rate_limit_violations_total", float64(response.Security.Violations.RateLimits))

	writePrometheusHelpType(&b, "olla_security_size_limit_violations_total", "counter", "Request size limit violations")
	writePrometheusGauge(&b, "olla_security_size_limit_violations_total", float64(response.Security.Violations.SizeLimits))

	writePrometheusHelpType(&b, "olla_security_status", "gauge", "Security posture (1=normal, 0=elevated)")
	writePrometheusGauge(&b, "olla_security_status", securityStatusValue(response.Security.Status))

	writeEndpointPrometheusMetrics(&b, snapshot.all, snapshot.endpointStats, snapshot.connectionStats, snapshot.endpointModels)

	_, _ = w.Write([]byte(b.String()))
}

func writeEndpointPrometheusMetrics(b *strings.Builder, all []*domain.Endpoint, statsMap map[string]ports.EndpointStats,
	connectionStats map[string]int64, modelMap map[string]*domain.EndpointModels) {
	writePrometheusHelpType(b, "olla_endpoint_up", "gauge", "Whether the endpoint is healthy (1) or not (0)")
	writePrometheusHelpType(b, "olla_endpoint_requests_total", "counter", "Total requests handled by endpoint")
	writePrometheusHelpType(b, "olla_endpoint_connections", "gauge", "Active connections for endpoint")
	writePrometheusHelpType(b, "olla_endpoint_success_rate_percent", "gauge", "Endpoint success rate percentage")
	writePrometheusHelpType(b, "olla_endpoint_avg_latency_ms", "gauge", "Endpoint average latency in milliseconds")
	writePrometheusHelpType(b, "olla_endpoint_traffic_bytes", "counter", "Total traffic for endpoint in bytes")
	writePrometheusHelpType(b, "olla_endpoint_priority", "gauge", "Endpoint routing priority")
	writePrometheusHelpType(b, "olla_endpoint_models_count", "gauge", "Number of models discovered on endpoint")

	for _, endpoint := range all {
		url := endpoint.GetURLString()
		stats, hasStats := statsMap[url]
		status := endpoint.Status.String()

		var successRate float64
		requests := int64(0)
		avgLatency := int64(0)
		trafficBytes := int64(0)
		if hasStats {
			requests = stats.TotalRequests
			avgLatency = stats.AverageLatency
			trafficBytes = stats.TotalBytes
			if stats.TotalRequests > 0 {
				successRate = float64(stats.SuccessfulRequests) / float64(stats.TotalRequests) * 100.0
			}
		}

		modelCount := int64(0)
		if endpointModels := modelMap[url]; endpointModels != nil {
			modelCount = int64(len(endpointModels.Models))
		}

		up := float64(0)
		if endpoint.Status == domain.StatusHealthy {
			up = 1
		}

		writePrometheusLabeledGauge(b, "olla_endpoint_up", up, "endpoint", endpoint.Name, "status", status)
		writePrometheusLabeledGauge(b, "olla_endpoint_requests_total", float64(requests), "endpoint", endpoint.Name, "status", status)
		writePrometheusLabeledGauge(b, "olla_endpoint_connections", float64(connectionStats[url]), "endpoint", endpoint.Name, "status", status)
		writePrometheusLabeledGauge(b, "olla_endpoint_success_rate_percent", successRate, "endpoint", endpoint.Name, "status", status)
		writePrometheusLabeledGauge(b, "olla_endpoint_avg_latency_ms", float64(avgLatency), "endpoint", endpoint.Name, "status", status)
		writePrometheusLabeledGauge(b, "olla_endpoint_traffic_bytes", float64(trafficBytes), "endpoint", endpoint.Name, "status", status)
		writePrometheusLabeledGauge(b, "olla_endpoint_priority", float64(endpoint.Priority), "endpoint", endpoint.Name, "status", status)
		writePrometheusLabeledGauge(b, "olla_endpoint_models_count", float64(modelCount), "endpoint", endpoint.Name, "status", status)
	}
}

func totalTrafficBytes(snapshot *statusSnapshot) int64 {
	var total int64
	for url, conn := range snapshot.connectionStats {
		_ = conn
		if stats, exists := snapshot.endpointStats[url]; exists {
			total += stats.TotalBytes
		}
	}
	return total
}

func systemStatusValue(status string) float64 {
	switch status {
	case statusHealthy:
		return 2
	case statusDegraded:
		return 1
	default:
		return 0
	}
}

func securityStatusValue(status string) float64 {
	if status == statusNormal {
		return 1
	}
	return 0
}

func parsePercentage(value string) float64 {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, "%")
	var parsed float64
	_, _ = fmt.Sscanf(value, "%f", &parsed)
	return parsed
}

func writePrometheusHelpType(b *strings.Builder, name, metricType, help string) {
	b.WriteString("# HELP ")
	b.WriteString(name)
	b.WriteByte(' ')
	b.WriteString(help)
	b.WriteByte('\n')
	b.WriteString("# TYPE ")
	b.WriteString(name)
	b.WriteByte(' ')
	b.WriteString(metricType)
	b.WriteByte('\n')
}

func writePrometheusGauge(b *strings.Builder, name string, value float64) {
	b.WriteString(name)
	b.WriteByte(' ')
	b.WriteString(formatPrometheusFloat(value))
	b.WriteByte('\n')
}

func writePrometheusLabeledGauge(b *strings.Builder, name string, value float64, labels ...string) {
	b.WriteString(name)
	b.WriteByte('{')
	for i := 0; i < len(labels); i += 2 {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(labels[i])
		b.WriteByte('=')
		b.WriteByte('"')
		b.WriteString(escapePrometheusLabel(labels[i+1]))
		b.WriteByte('"')
	}
	b.WriteByte('}')
	b.WriteByte(' ')
	b.WriteString(formatPrometheusFloat(value))
	b.WriteByte('\n')
}

func escapePrometheusLabel(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	return value
}

func formatPrometheusFloat(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", value), "0"), ".")
}
