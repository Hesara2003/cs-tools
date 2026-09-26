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
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakePinger struct {
	err error
}

func (f fakePinger) Health(ctx context.Context) error {
	return f.err
}

func TestHealthHandler_GetHealthDependencies_AllHealthy(t *testing.T) {
	h := NewHealthHandler(fakePinger{}, fakePinger{}, fakePinger{}, fakePinger{}, fakePinger{})

	w := httptest.NewRecorder()
	h.GetHealthDependencies(w, httptest.NewRequest(http.MethodGet, "/health/dependencies", nil))

	assertStatus(t, w, http.StatusOK)
	resp := decodeJSON[HealthDependenciesResponse](t, w)
	if resp.Status != "ok" {
		t.Errorf("Status = %q, want ok", resp.Status)
	}
	if len(resp.Dependencies) != 5 {
		t.Fatalf("len(Dependencies) = %d, want 5", len(resp.Dependencies))
	}
	for _, d := range resp.Dependencies {
		if d.Status != "ok" {
			t.Errorf("%s status = %q, want ok", d.Name, d.Status)
		}
	}
}

func TestHealthHandler_GetHealthDependencies_OneDown(t *testing.T) {
	h := NewHealthHandler(fakePinger{}, fakePinger{err: errors.New("boom")}, fakePinger{}, fakePinger{}, fakePinger{})

	w := httptest.NewRecorder()
	h.GetHealthDependencies(w, httptest.NewRequest(http.MethodGet, "/health/dependencies", nil))

	assertStatus(t, w, http.StatusServiceUnavailable)
	resp := decodeJSON[HealthDependenciesResponse](t, w)
	if resp.Status != "degraded" {
		t.Errorf("Status = %q, want degraded", resp.Status)
	}
	var sawUpdatesDown bool
	for _, d := range resp.Dependencies {
		if d.Name == "Updates Service" && d.Status == "down" {
			sawUpdatesDown = true
		}
	}
	if !sawUpdatesDown {
		t.Error("expected Updates Service to be reported down")
	}
}

func TestHealthHandler_GetHealthDependencies_NotConfiguredIsNotDown(t *testing.T) {
	// notification/integration/engineering nil (unconfigured) must never drag
	// the overall status into "degraded" — that's a deployment choice, not a
	// dependency failure.
	h := NewHealthHandler(fakePinger{}, fakePinger{}, nil, nil, nil)

	w := httptest.NewRecorder()
	h.GetHealthDependencies(w, httptest.NewRequest(http.MethodGet, "/health/dependencies", nil))

	assertStatus(t, w, http.StatusOK)
	resp := decodeJSON[HealthDependenciesResponse](t, w)
	if resp.Status != "ok" {
		t.Errorf("Status = %q, want ok", resp.Status)
	}
	for _, d := range resp.Dependencies {
		switch d.Name {
		case "CSM Notification Service", "CSM Integration Service", "Engineering Entity Service":
			if d.Status != "not_configured" {
				t.Errorf("%s status = %q, want not_configured", d.Name, d.Status)
			}
		}
	}
}
