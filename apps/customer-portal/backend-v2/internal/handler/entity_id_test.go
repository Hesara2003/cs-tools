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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wso2-open-operations/cs-tools/apps/customer-portal/backend-v2/internal/entity"
)

// The same synthetic identifier in both shapes it reaches this API as. A
// dashless id like this one, on a real detail route, used to answer
// {"message":"Invalid UUID format."} — the handler required the dashed form,
// so entity-service never saw the request at all.
const (
	testDashlessID = "a1b2c3d4e5f60718293a4b5c6d7e8f90"
	testDashedID   = "a1b2c3d4-e5f6-0718-293a-4b5c6d7e8f90"
)

// TestToDashedID pins the normalization every id path parameter now runs
// through. The dashed form is what gets forwarded upstream because it is the
// only shape both entity-service data sources accept — ServiceNow strips the
// hyphens itself, Postgres validates them.
func TestToDashedID(t *testing.T) {
	tests := map[string]struct {
		in   string
		want string
	}{
		"bare sysid gains hyphens":      {in: testDashlessID, want: testDashedID},
		"uppercase sysid keeps case":    {in: "A1B2C3D4E5F60718293A4B5C6D7E8F90", want: "A1B2C3D4-E5F6-0718-293A-4B5C6D7E8F90"},
		"dashed uuid is untouched":      {in: testDashedID, want: testDashedID},
		"empty is untouched":            {in: "", want: ""},
		"31 hex is untouched":           {in: "a1b2c3d4e5f60718293a4b5c6d7e8f9", want: "a1b2c3d4e5f60718293a4b5c6d7e8f9"},
		"33 hex is untouched":           {in: "a1b2c3d4e5f60718293a4b5c6d7e8f900", want: "a1b2c3d4e5f60718293a4b5c6d7e8f900"},
		"non-hex is untouched":          {in: "a1b2c3d4e5f60718293a4b5c6d7e8fzz", want: "a1b2c3d4e5f60718293a4b5c6d7e8fzz"},
		"path traversal is untouched":   {in: "../../secrets", want: "../../secrets"},
		"already-dashed junk untouched": {in: "not-an-id", want: "not-an-id"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := toDashedID(tc.in); got != tc.want {
				t.Fatalf("toDashedID(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestIsEntityID covers the widened predicate: both id shapes are accepted,
// and anything that is neither is still refused so it can never reach an
// upstream URL path.
func TestIsEntityID(t *testing.T) {
	valid := []string{
		testDashedID,
		testDashlessID,
		"A1B2C3D4-E5F6-0718-293A-4B5C6D7E8F90",
		"A1B2C3D4E5F60718293A4B5C6D7E8F90",
	}
	for _, id := range valid {
		if !isEntityID(id) {
			t.Errorf("isEntityID(%q) = false, want true", id)
		}
	}

	invalid := []string{
		"",
		"   ",
		"a1b2c3d4",
		"a1b2c3d4e5f60718293a4b5c6d7e8f9",   // 31 hex
		"a1b2c3d4e5f60718293a4b5c6d7e8f900", // 33 hex
		"a1b2c3d4e5f60718293a4b5c6d7e8fzz",  // non-hex
		"a1b2c3d4-e5f6-0718-4b5c6d7e8f90",   // missing a group
		"../../secrets",
		testDashedID + "/../other",
		testDashedID + "?x=1",
	}
	for _, id := range invalid {
		if isEntityID(id) {
			t.Errorf("isEntityID(%q) = true, want false", id)
		}
	}
}

// fakeEntityProjectClient records the id GetProject was called with.
// entityProjectClient is embedded (nil) so only the method under test needs
// implementing.
type fakeEntityProjectClient struct {
	entityProjectClient
	gotID  string
	called bool
}

func (f *fakeEntityProjectClient) GetProject(ctx context.Context, id string) (entity.ProjectDetailsView, error) {
	f.gotID = id
	f.called = true
	return entity.ProjectDetailsView{ID: id}, nil
}

// TestGetProject_AcceptsDashlessID is the regression test for the reported
// staging 400. It goes through a real http.ServeMux registered with the exact
// pattern main.go uses, and asserts both halves of the fix: the dashless id is
// no longer rejected, and what reaches entity-service is the dashed form (a
// dashless id forwarded verbatim would work on a ServiceNow deployment and 400
// on a Postgres one, whose GetProjectByID runs validateUUIDs).
func TestGetProject_AcceptsDashlessID(t *testing.T) {
	tests := map[string]struct {
		id         string
		wantStatus int
		wantSentID string
	}{
		"dashless sysid": {id: testDashlessID, wantStatus: http.StatusOK, wantSentID: testDashedID},
		"dashed uuid":    {id: testDashedID, wantStatus: http.StatusOK, wantSentID: testDashedID},
		"neither shape":  {id: "a1b2c3d4", wantStatus: http.StatusBadRequest},
		"path traversal": {id: "..%2F..%2Fsecrets", wantStatus: http.StatusBadRequest},
		"trailing junk":  {id: testDashlessID + "x", wantStatus: http.StatusBadRequest},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fake := &fakeEntityProjectClient{}
			h := NewProjectHandler(fake)

			mux := http.NewServeMux()
			mux.HandleFunc("GET /projects/{id}", h.GetProject)

			req := authedRequest(http.MethodGet, "/projects/"+tc.id, "")
			rec := httptest.NewRecorder()

			mux.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantStatus != http.StatusOK {
				if fake.called {
					t.Errorf("entity-service GetProject was called with a malformed id (%q)", fake.gotID)
				}
				return
			}
			if fake.gotID != tc.wantSentID {
				t.Fatalf("entity-service GetProject got id %q, want %q", fake.gotID, tc.wantSentID)
			}
		})
	}
}
