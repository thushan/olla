package domain

import (
	"fmt"
	"time"
)

type EndpointState string

const (
	EndpointStateUnknown  EndpointState = "unknown"
	EndpointStateOnline   EndpointState = "online"
	EndpointStateDegraded EndpointState = "degraded"
	EndpointStateOffline  EndpointState = "offline"
	EndpointStateRemoved  EndpointState = "removed"
)

type ModelState string

const (
	ModelStateUnknown   ModelState = "unknown"
	ModelStateAvailable ModelState = "available"
	ModelStateLoaded    ModelState = "loaded"
	ModelStateLoading   ModelState = "loading"
	ModelStateOffline   ModelState = "offline"
	ModelStateError     ModelState = "error"
)

type EndpointStateInfo struct {
	LastStateChange     time.Time              `json:"last_state_change"`
	Metadata            map[string]interface{} `json:"metadata,omitempty"`
	State               EndpointState          `json:"state"`
	LastError           string                 `json:"last_error,omitempty"`
	ConsecutiveFailures int                    `json:"consecutive_failures"`
}

type StateTransition struct {
	Timestamp time.Time `json:"timestamp"`
	Error     error     `json:"error,omitempty"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Reason    string    `json:"reason"`
}

func (s EndpointState) IsHealthy() bool {
	return s == EndpointStateOnline || s == EndpointStateDegraded
}

func (s EndpointState) IsTerminal() bool {
	return s == EndpointStateRemoved
}

// CanTransitionTo enforces the state machine rules for endpoint lifecycle
func (s EndpointState) CanTransitionTo(target EndpointState) bool {
	if s == EndpointStateRemoved {
		return false
	}

	if target == EndpointStateRemoved {
		return true
	}

	validTransitions := map[EndpointState][]EndpointState{
		EndpointStateUnknown:  {EndpointStateOnline, EndpointStateOffline, EndpointStateDegraded},
		EndpointStateOnline:   {EndpointStateOffline, EndpointStateDegraded, EndpointStateUnknown},
		EndpointStateDegraded: {EndpointStateOnline, EndpointStateOffline, EndpointStateUnknown},
		EndpointStateOffline:  {EndpointStateOnline, EndpointStateDegraded, EndpointStateUnknown},
	}

	allowed, exists := validTransitions[s]
	if !exists {
		return false
	}

	for _, state := range allowed {
		if state == target {
			return true
		}
	}
	return false
}

func (s ModelState) IsHealthy() bool {
	return s == ModelStateAvailable || s == ModelStateLoaded || s == ModelStateLoading
}

func (s EndpointState) String() string {
	return string(s)
}

func (s ModelState) String() string {
	return string(s)
}

func (s EndpointState) Validate() error {
	switch s {
	case EndpointStateUnknown, EndpointStateOnline, EndpointStateDegraded,
		EndpointStateOffline, EndpointStateRemoved:
		return nil
	default:
		return fmt.Errorf("invalid endpoint state: %s", s)
	}
}

func (s ModelState) Validate() error {
	switch s {
	case ModelStateUnknown, ModelStateAvailable, ModelStateLoaded,
		ModelStateLoading, ModelStateOffline, ModelStateError:
		return nil
	default:
		return fmt.Errorf("invalid model state: %s", s)
	}
}
