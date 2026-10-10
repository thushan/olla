package handlers

// Patch #3 — runtime endpoint drain/undrain (design: meta/architecture/2026-10-10-olla-drain-lifecycle.md).
// POST /internal/endpoints/{name}/drain   body {"reason": "..."} optional
// POST /internal/endpoints/{name}/undrain
// Drain state is in-memory (Endpoint.Drained); the health checker preserves it.

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/thushan/olla/internal/core/domain"
)

func (a *Application) drainHandler(w http.ResponseWriter, r *http.Request) {
	a.drainToggle(w, r, true)
}

func (a *Application) undrainHandler(w http.ResponseWriter, r *http.Request) {
	a.drainToggle(w, r, false)
}

func (a *Application) drainToggle(w http.ResponseWriter, r *http.Request, drained bool) {
	name := strings.TrimPrefix(r.URL.Path, "/internal/endpoints/")
	name = strings.TrimSuffix(strings.TrimSuffix(name, "/drain"), "/undrain")
	ctx := r.Context()

	eps, err := a.repository.GetAll(ctx)
	if err != nil {
		http.Error(w, "repository error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var target *domain.Endpoint
	for _, e := range eps {
		if strings.EqualFold(e.Name, name) {
			target = e
			break
		}
	}
	if target == nil {
		http.Error(w, "unknown endpoint: "+name, http.StatusNotFound)
		return
	}

	reason := ""
	if drained && r.ContentLength > 0 {
		var body struct {
			Reason string `json:"reason"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		reason = body.Reason
	}

	// Mutate the stored endpoint through UpdateEndpoint so LB + checker see it.
	updated := *target
	updated.Drained = drained
	updated.DrainReason = reason
	if drained {
		updated.DrainedAt = time.Now()
	} else {
		updated.DrainedAt = time.Time{}
		updated.DrainReason = ""
	}
	if err := a.repository.UpdateEndpoint(ctx, &updated); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	state := "undrained"
	if drained {
		state = "drained"
	}
	a.logger.Info("Endpoint drain state changed", "endpoint", target.Name, "state", state, "reason", reason)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"endpoint": target.Name, "state": state, "reason": reason,
		"status": string(updated.Status), "was_healthy": updated.Status == domain.StatusHealthy,
	})
}
