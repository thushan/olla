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
  // Proxy-wide latency rollup (WP-A3): plain int64 on the Go side, no
  // omitempty, so both are always present - an idle fleet serialises bare
  // zeros rather than omitting the fields.
  min_latency_ms: number;
  max_latency_ms: number;
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
