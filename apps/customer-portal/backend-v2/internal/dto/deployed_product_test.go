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

package dto

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/wso2-open-operations/cs-tools/apps/customer-portal/backend-v2/internal/entity"
)

func TestToSysID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{
			input: "4e8431b1-1b8c-0310-0bb3-da47b04bcba6",
			want:  "4e8431b11b8c03100bb3da47b04bcba6",
		},
		{
			input: "4e8431b11b8c03100bb3da47b04bcba6",
			want:  "4e8431b11b8c03100bb3da47b04bcba6",
		},
		{
			input: "",
			want:  "",
		},
	}
	for _, tc := range tests {
		if got := ToSysID(tc.input); got != tc.want {
			t.Errorf("ToSysID(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestBuildEntitySearchDeployedProductsRequest(t *testing.T) {
	t.Run("converts dashed UUID to sysid and sets filters", func(t *testing.T) {
		req := DeployedProductSearchRequest{
			Pagination: entity.Pagination{Limit: 10, Offset: 0},
			Filters: &DeployedProductSearchFilters{
				ProductCategories: []string{"Integration"},
			},
		}
		got := BuildEntitySearchDeployedProductsRequest("4e8431b1-1b8c-0310-0bb3-da47b04bcba6", req)

		if got.Filters == nil {
			t.Fatal("expected Filters to be non-nil")
		}
		expectedSysID := "4e8431b11b8c03100bb3da47b04bcba6"
		if len(got.Filters.DeploymentIDs) != 1 || got.Filters.DeploymentIDs[0] != expectedSysID {
			t.Errorf("got DeploymentIDs = %v, want [%s]", got.Filters.DeploymentIDs, expectedSysID)
		}
		if !reflect.DeepEqual(got.Filters.ProductCategories, []string{"Integration"}) {
			t.Errorf("got ProductCategories = %v, want [Integration]", got.Filters.ProductCategories)
		}

		data, err := json.Marshal(got)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}
		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			t.Fatalf("json.Unmarshal failed: %v", err)
		}
		if _, hasRootDepID := raw["deploymentIds"]; hasRootDepID {
			t.Errorf("expected no deploymentIds at root of serialized request, got: %v", raw)
		}
		filtersMap, ok := raw["filters"].(map[string]any)
		if !ok {
			t.Fatalf("expected filters object in serialized json, got: %v", raw)
		}
		if deps, ok := filtersMap["deploymentIds"].([]any); !ok || len(deps) != 1 || deps[0] != expectedSysID {
			t.Errorf("expected filters.deploymentIds to contain %s, got: %v", expectedSysID, filtersMap["deploymentIds"])
		}
	})

	t.Run("bare sysid without category filters", func(t *testing.T) {
		req := DeployedProductSearchRequest{
			Pagination: entity.Pagination{Limit: 20, Offset: 10},
		}
		got := BuildEntitySearchDeployedProductsRequest("4e8431b11b8c03100bb3da47b04bcba6", req)
		if got.Filters == nil {
			t.Fatal("expected Filters to be non-nil")
		}
		if len(got.Filters.DeploymentIDs) != 1 || got.Filters.DeploymentIDs[0] != "4e8431b11b8c03100bb3da47b04bcba6" {
			t.Errorf("got DeploymentIDs = %v", got.Filters.DeploymentIDs)
		}
		if len(got.Filters.ProductCategories) != 0 {
			t.Errorf("expected empty ProductCategories, got %v", got.Filters.ProductCategories)
		}
	})
}

func TestUnmarshalSearchDeployedProductsResponse_UpstreamBallerinaPayload(t *testing.T) {
	rawJSON := `{
		"deployedProducts": [
			{
				"id": "dp123456789012345678901234567890",
				"deployment": {"id": "d1", "name": "Prod"},
				"product": {"id": "p1", "name": "APIM"},
				"version": {
					"id": "v1",
					"name": "4.2.0",
					"releasedOn": "2023-01-15",
					"endOfLifeOn": "2026-01-15"
				},
				"cores": 8,
				"tps": 250.5,
				"category": {
					"id": "cat1",
					"name": "API Management"
				},
				"createdOn": "2024-02-10 09:30:00",
				"updatedOn": "2024-02-11 10:00:00"
			}
		],
		"totalRecords": 1,
		"offset": 0,
		"limit": 10
	}`

	var entityResp entity.SearchDeployedProductsResponse
	if err := json.Unmarshal([]byte(rawJSON), &entityResp); err != nil {
		t.Fatalf("failed to unmarshal upstream ballerina response: %v", err)
	}

	if entityResp.Total != 1 {
		t.Errorf("entityResp.Total = %d, want 1", entityResp.Total)
	}
	if len(entityResp.DeployedProducts) != 1 {
		t.Fatalf("expected 1 deployed product, got %d", len(entityResp.DeployedProducts))
	}
	dp := entityResp.DeployedProducts[0]
	if dp.Cores == nil || *dp.Cores != "8" {
		t.Errorf("dp.Cores = %v, want '8'", dp.Cores)
	}
	if dp.TPS == nil || *dp.TPS != "250.5" {
		t.Errorf("dp.TPS = %v, want '250.5'", dp.TPS)
	}
	if dp.Category == nil || *dp.Category != "API Management" {
		t.Errorf("dp.Category = %v, want 'API Management'", dp.Category)
	}
	if dp.Version == nil || dp.Version.ReleasedDate == nil {
		t.Fatalf("expected version releasedDate to be parsed")
	}
	expectedReleased := time.Date(2023, 1, 15, 0, 0, 0, 0, time.UTC)
	if !dp.Version.ReleasedDate.Equal(expectedReleased) {
		t.Errorf("version.ReleasedDate = %v, want %v", dp.Version.ReleasedDate, expectedReleased)
	}

	// Now map to portal response
	portalResp := MapSearchDeployedProducts(entityResp)
	if portalResp.TotalRecords != 1 {
		t.Errorf("portalResp.TotalRecords = %d, want 1", portalResp.TotalRecords)
	}
	if len(portalResp.DeployedProducts) != 1 {
		t.Fatalf("expected 1 portal deployed product, got %d", len(portalResp.DeployedProducts))
	}
	summary := portalResp.DeployedProducts[0]
	if summary.Cores == nil || *summary.Cores != 8 {
		t.Errorf("summary.Cores = %v, want 8", summary.Cores)
	}
	if summary.TPS == nil || *summary.TPS != 250.5 {
		t.Errorf("summary.TPS = %v, want 250.5", summary.TPS)
	}
	if summary.Category == nil || *summary.Category != "API Management" {
		t.Errorf("summary.Category = %v, want 'API Management'", summary.Category)
	}
}

func TestBuildEntityCreateDeployedProductRequest(t *testing.T) {
	cores := 4
	tps := 100.0
	desc := "test product"
	req := DeployedProductCreateRequest{
		ProductID:   "5e8431b1-1b8c-0310-0bb3-da47b04bcba6",
		VersionID:   "6e8431b1-1b8c-0310-0bb3-da47b04bcba6",
		ProjectID:   "7e8431b1-1b8c-0310-0bb3-da47b04bcba6",
		Cores:       &cores,
		TPS:         &tps,
		Description: &desc,
	}
	got := BuildEntityCreateDeployedProductRequest("4e8431b1-1b8c-0310-0bb3-da47b04bcba6", req)

	if got.DeploymentID != "4e8431b11b8c03100bb3da47b04bcba6" {
		t.Errorf("got DeploymentID = %q, want %q", got.DeploymentID, "4e8431b11b8c03100bb3da47b04bcba6")
	}
	if got.ProductID != "5e8431b11b8c03100bb3da47b04bcba6" {
		t.Errorf("got ProductID = %q, want %q", got.ProductID, "5e8431b11b8c03100bb3da47b04bcba6")
	}
	if got.VersionID != "6e8431b11b8c03100bb3da47b04bcba6" {
		t.Errorf("got VersionID = %q, want %q", got.VersionID, "6e8431b11b8c03100bb3da47b04bcba6")
	}
	if got.ProjectID != "7e8431b11b8c03100bb3da47b04bcba6" {
		t.Errorf("got ProjectID = %q, want %q", got.ProjectID, "7e8431b11b8c03100bb3da47b04bcba6")
	}
}

func TestBuildEntityUpdateDeployedProductRequest(t *testing.T) {
	cores := 2
	req := DeployedProductUpdateRequest{
		Cores: &cores,
	}
	got := BuildEntityUpdateDeployedProductRequest("1e8431b1-1b8c-0310-0bb3-da47b04bcba6", "2e8431b1-1b8c-0310-0bb3-da47b04bcba6", req)

	if got.ID != "1e8431b11b8c03100bb3da47b04bcba6" {
		t.Errorf("got ID = %q, want %q", got.ID, "1e8431b11b8c03100bb3da47b04bcba6")
	}
	if got.DeploymentID == nil || *got.DeploymentID != "2e8431b11b8c03100bb3da47b04bcba6" {
		t.Errorf("got DeploymentID = %v, want 2e8431b11b8c03100bb3da47b04bcba6", got.DeploymentID)
	}
}
