package constants

// Routing reason constants used across all routing strategies
// these define the reasons for routing decisions that are returned
// in the X-Olla-Routing-Reason header and determine HTTP status codes
const (
	// Model found scenarios (200 OK)
	RoutingReasonModelFound          = "model_found"
	RoutingReasonModelFoundNoRefresh = "model_found_no_refresh"

	// Model not found scenarios (404 Not Found)
	RoutingReasonModelNotFound           = "model_not_found"
	RoutingReasonModelNotFoundFallback   = "model_not_found_fallback"
	RoutingReasonNoHealthyAfterDiscovery = "no_healthy_after_discovery"

	// Model unavailable scenarios (503 Service Unavailable)
	RoutingReasonModelUnavailable               = "model_unavailable"
	RoutingReasonModelUnavailableNoFallback     = "model_unavailable_no_fallback"
	RoutingReasonModelUnavailableCompatibleOnly = "model_unavailable_compatible_only"
	RoutingReasonModelUnavailableNoRefresh      = "model_unavailable_no_refresh"
	RoutingReasonModelUnavailableAfterDiscovery = "model_unavailable_after_discovery"

	// Fallback scenarios (200 OK with fallback action)
	RoutingReasonAllHealthyFallback       = "all_healthy_fallback"
	RoutingReasonAllHealthyAfterDiscovery = "all_healthy_after_discovery"

	// Discovery-specific scenarios
	RoutingReasonDiscoveryFailedNoFallback     = "discovery_failed_no_fallback"
	RoutingReasonDiscoveryFailedCompatibleOnly = "discovery_failed_compatible_only"
	RoutingReasonDiscoveryFailedAllFallback    = "discovery_failed_all_fallback"
	RoutingReasonDiscoveryErrorFallback        = "discovery_error_fallback"
	RoutingReasonDiscoveryError                = "discovery_error"
)

// Fallback behavior constants for routing strategies
const (
	// FallbackBehaviorNone never falls back to other endpoints
	FallbackBehaviorNone = "none"

	// FallbackBehaviorCompatibleOnly only uses endpoints known to support the model
	FallbackBehaviorCompatibleOnly = "compatible_only"

	// FallbackBehaviorAll falls back to any healthy endpoint
	FallbackBehaviorAll = "all"
)
