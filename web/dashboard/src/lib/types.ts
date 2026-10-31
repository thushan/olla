// API contracts for the three status surfaces, hand-mirrored from the Go
// handler response structs. Types only, no runtime code. Keep this in
// lockstep with:
//   - internal/app/handlers/handler_status.go
//   - internal/app/handlers/handler_status_endpoints.go
//   - internal/app/handlers/handler_status_models.go
//
// Discipline: time.Time serialises via encoding/json as an ISO 8601 string,
// so every time.Time becomes string here. Pointer fields and omitempty
// fields become optional; plain fields with no omitempty stay required.

// Poll lifecycle state shared by every store. Lifted here so the stores can
// pin PollStore<StatusResponse> etc. against a single source of truth.
export type PollStatus = 'loading' | 'ok' | 'error' | 'stale';

// --- handler_status.go -------------------------------------------------

export interface SecurityViolation {
  rate_limits: number;
  size_limits: number;
}

export interface SecuritySummary {
  status: string;
  blocked_ips: number;
  violations: SecurityViolation;
}

export interface ProxySummary {
  engine: string;
  profile: string;
  balancer: string;
}

export interface SystemSummary {
  start_time: string;
  status: string;
  endpoints_up: string;
  success_rate: string;
  avg_latency: string;
  total_traffic: string;
  uptime: string;
  version: string;
  commit: string;
  active_connections: number;
  security_violations: number;
  total_requests: number;
  total_failures: number;
  // Lets the dashboard branch on the no-traffic state without guessing from
  // a "0%" success rate. Always emitted by the backend (no omitempty); false
  // on a fresh boot, flips true once any proxy request has been counted.
  // When false, success_rate arrives as "N/A" rather than "0%".
  has_traffic: boolean;
}

export interface EndpointModelsResponse {
  last_updated: string;
  count: number;
}

export interface EndpointResponse {
  avg_latency_ms?: number;
  next_check_at?: string;
  health_check_at?: string;
  models: EndpointModelsResponse;
  name: string;
  status: string;
  success_rate: string;
  avg_latency: string;
  traffic: string;
  last_check: string;
  next_check: string;
  issues: string;
  url: string;
  id: string;
  priority: number;
  connections: number;
  requests: number;
  min_latency_ms: number;
  max_latency_ms: number;
}

export interface StatusResponse {
  timestamp: string;
  proxy: ProxySummary;
  endpoints: EndpointResponse[];
  security: SecuritySummary;
  system: SystemSummary;
}

// --- handler_status_endpoints.go --------------------------------------

export interface EndpointSummary {
  avg_latency_ms?: number;
  next_check_at?: string;
  health_check_at?: string;
  last_model_sync_at?: string;
  name: string;
  type: string;
  status: string;
  last_model_sync?: string;
  health_check: string;
  response_time?: string;
  success_rate: string;
  issues?: string;
  url: string;
  id: string;
  priority: number;
  model_count: number;
  request_count: number;
  min_latency_ms: number;
  max_latency_ms: number;
  active_connections: number;
}

export interface EndpointStatusResponse {
  timestamp: string;
  endpoints: EndpointSummary[];
  total_count: number;
  healthy_count: number;
  routable_count: number;
}

// --- handler_status_models.go ----------------------------------------

export interface ModelSummary {
  last_seen_at?: string;
  name: string;
  type?: string;
  family?: string;
  size?: string;
  params?: string;
  quant?: string;
  last_seen: string;
  endpoints: string[];
  // Positional pair of `endpoints`: endpoints[i] is hosted on endpoint_ids[i].
  // The backend emits both omitempty, so either may be absent on older builds;
  // when present the ids are the keys to EndpointsPanel's ep-${stableId(id)}
  // row ids (display names are NOT unique - see EndpointsPanel.dup-names.test).
  endpoint_ids?: string[];
  aliases?: string[];
  capabilities?: string[];
}

export interface ModelGroupSummary {
  family: string;
  models: ModelSummary[];
  endpoints: string[];
  model_count: number;
}

export interface ModelStatusResponse {
  timestamp: string;
  models_by_family: Record<string, string[]>;
  recent_models: ModelSummary[];
  model_groups?: ModelGroupSummary[];
  total_models: number;
  total_families: number;
  total_endpoints: number;
}

// /internal/stats/sticky. When sticky sessions are off the handler answers
// 200 {"enabled":false} and nothing else, so every other field is optional.
// Counters are raw numbers.
export interface StickyStatsResponse {
  enabled: boolean;
  active_sessions?: number;
  insertions?: number;
  hits?: number;
  misses?: number;
  evictions?: number;
  max_sessions?: number;
  idle_ttl_seconds?: number;
}

// /internal/stats/translators. Counts are numbers; rates, success and latency
// arrive as pre-formatted strings ("42.5%", "1.2s").
export interface TranslatorStatsEntry {
  translator_name: string;
  success_rate: string;
  passthrough_rate: string;
  average_latency: string;
  total_requests: number;
  successful_requests: number;
  failed_requests: number;
  passthrough_requests: number;
  translation_requests: number;
  streaming_requests: number;
  non_streaming_requests: number;
  fallback_no_compatible_endpoints: number;
  fallback_translator_does_not_support_passthrough: number;
  fallback_cannot_passthrough: number;
}

export interface TranslatorStatsResponse {
  timestamp: string;
  translators: TranslatorStatsEntry[];
  summary: {
    overall_success_rate: string;
    overall_passthrough_rate: string;
    total_translators: number;
    active_translators: number;
    total_requests: number;
    total_passthrough: number;
    total_translations: number;
    total_streaming: number;
    total_non_streaming: number;
  };
}

// /internal/process. Memory sizes, durations and the pressure/health labels are
// pre-formatted strings; goroutine count and GC cycles are numbers. avg_gc_pause
// is "" until the first GC has run.
export interface ProcessStatsResponse {
  timestamp: string;
  memory: {
    heap_alloc: string;
    heap_sys: string;
    heap_inuse: string;
    memory_pressure: string;
  };
  garbage_collection: {
    avg_gc_pause: string;
    num_gc_cycles: number;
  };
  goroutines: { health_status: string; count: number };
  runtime: { uptime: string; go_version: string; num_cpu: number };
}
