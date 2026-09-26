// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// HealthPinger is satisfied by any upstream client that exposes a health
// check — scim.Client, updates.Client, csmnotification.Client and
// csmintegration.Client all do.
type HealthPinger interface {
	Health(ctx context.Context) error
}

// dependencyHealthCheckTimeout bounds each individual dependency check so one
// slow or hanging upstream cannot make the whole aggregation hang.
const dependencyHealthCheckTimeout = 5 * time.Second

// Dependency health status vocabulary for HealthDependenciesResponse.
const (
	dependencyStatusOK            = "ok"
	dependencyStatusDown          = "down"
	dependencyStatusNotConfigured = "not_configured"
)

// DependencyHealth is one entry of HealthDependenciesResponse. It carries no
// failure detail by design — this route is unauthenticated and public, the
// same posture entity-service's own GET /health/database documents ("failure
// bodies carry no detail... this endpoint is unauthenticated and public").
type DependencyHealth struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// HealthDependenciesResponse is the body of GET /health/dependencies.
type HealthDependenciesResponse struct {
	Status       string             `json:"status"`
	Dependencies []DependencyHealth `json:"dependencies"`
}

// HealthHandler serves GET /health/dependencies — an aggregating check over
// this backend's own upstream integrations, distinct from the bare
// GET /health liveness probe registered directly on the mux in
// cmd/server/main.go. The two are kept separate on purpose, mirroring
// entity-service's own GET /health vs GET /health/database split: a
// liveness probe that fails because SCIM or Updates is briefly down would
// have the orchestrator restart or drain an instance that is otherwise
// working fine. This endpoint is for monitoring/on-call visibility; do not
// wire it up as the restart-triggering probe.
//
// entity-service is deliberately not checked here — this backend depends on
// it for nearly every request, and checking it is out of scope for this
// aggregation by explicit product decision. Engineering Entity Service is
// also not checked, and not listed at all: it has no health endpoint of its
// own anywhere in its repo, so there's nothing here to call — reporting a
// fixed "unknown" for it was tried first and dropped as noise, since it can
// never be anything else until that service adds one.
type HealthHandler struct {
	scim         HealthPinger
	updates      HealthPinger
	notification HealthPinger // nil when CSM_NOTIFICATION_SERVICE_BASE_URL is unset
	integration  HealthPinger // nil when CSM_INTEGRATION_SERVICE_BASE_URL is unset
}

// NewHealthHandler constructs a HealthHandler. notification/integration may
// be nil (pass an untyped nil, never a nil-valued concrete pointer — see
// entity-service's own CLAUDE.md for why a typed nil boxed into an interface
// is a common bug here) when that service's base URL is not configured.
func NewHealthHandler(scim, updates, notification, integration HealthPinger) *HealthHandler {
	return &HealthHandler{
		scim:         scim,
		updates:      updates,
		notification: notification,
		integration:  integration,
	}
}

// GetHealthDependencies checks every configured dependency concurrently and
// reports 200 when all are ok, 503 when any is down. No auth check — this
// route is registered directly on the mux and is exempt from the Auth
// middleware, the same way GET /health is (see cmd/server/main.go and
// internal/middleware/auth.go).
func (h *HealthHandler) GetHealthDependencies(w http.ResponseWriter, r *http.Request) {
	checks := []struct {
		name   string
		pinger HealthPinger
	}{
		{"scim", h.scim},
		{"updates", h.updates},
		{"csm-notification-service", h.notification},
		{"csm-integration-service", h.integration},
	}

	deps := make([]DependencyHealth, len(checks))
	var wg sync.WaitGroup
	for i, c := range checks {
		wg.Add(1)
		go func(i int, name string, pinger HealthPinger) {
			defer wg.Done()
			deps[i] = checkDependency(r.Context(), name, pinger)
		}(i, c.name, c.pinger)
	}
	wg.Wait()

	overall := dependencyStatusOK
	statusCode := http.StatusOK
	for _, d := range deps {
		if d.Status == dependencyStatusDown {
			overall = "degraded"
			statusCode = http.StatusServiceUnavailable
			break
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(HealthDependenciesResponse{Status: overall, Dependencies: deps})
}

func checkDependency(ctx context.Context, name string, pinger HealthPinger) DependencyHealth {
	if pinger == nil {
		return DependencyHealth{Name: name, Status: dependencyStatusNotConfigured}
	}
	ctx, cancel := context.WithTimeout(ctx, dependencyHealthCheckTimeout)
	defer cancel()
	if err := pinger.Health(ctx); err != nil {
		return DependencyHealth{Name: name, Status: dependencyStatusDown}
	}
	return DependencyHealth{Name: name, Status: dependencyStatusOK}
}
