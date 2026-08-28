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
	"strings"
	"testing"

	"github.com/wso2-open-operations/cs-tools/apps/customer-portal/backend-v2/internal/entity"
)

const callerScopeTestProjectID = "22222222-2222-2222-2222-222222222222"
const callerScopeTestEmail = "customer@example.com"

// fakeEntityContacts implements entityProjectContactsClient, keyed by
// project id.
type fakeEntityContacts struct {
	byProjectID    map[string][]entity.ProjectContact
	errByProjectID map[string]error
	// pages, if set, overrides byProjectID for pagination tests: each call
	// pops the next entry, keyed by offset.
	pagesByProjectID map[string]map[int]entity.SearchProjectContactsResponse
}

func (f *fakeEntityContacts) SearchProjectContacts(_ context.Context, projectID string, req entity.SearchProjectContactsRequest) (entity.SearchProjectContactsResponse, error) {
	if err, ok := f.errByProjectID[projectID]; ok {
		return entity.SearchProjectContactsResponse{}, err
	}
	if pages, ok := f.pagesByProjectID[projectID]; ok {
		return pages[req.Pagination.Offset], nil
	}
	contacts := f.byProjectID[projectID]
	return entity.SearchProjectContactsResponse{Contacts: contacts, Total: len(contacts)}, nil
}

// fakeEntityProjectResolver implements entityProjectClient (SearchProjects +
// GetProject) for the ProjectHandler.SearchProjects tests.
type fakeEntityProjectResolver struct {
	searchResult entity.SearchProjectsResponse
	searchErr    error
}

func (f *fakeEntityProjectResolver) GetProject(_ context.Context, id string) (entity.ProjectDetailsView, error) {
	return entity.ProjectDetailsView{ID: id}, nil
}

func (f *fakeEntityProjectResolver) SearchProjects(_ context.Context, _ entity.SearchProjectsRequest) (entity.SearchProjectsResponse, error) {
	return f.searchResult, f.searchErr
}

func TestCallerScopeResolver_IsProjectMember(t *testing.T) {
	t.Run("true for a matching contact that grants case access, case-insensitive", func(t *testing.T) {
		fake := &fakeEntityContacts{byProjectID: map[string][]entity.ProjectContact{
			callerScopeTestProjectID: {{Email: "Customer@Example.com", GrantsCaseAccess: true}},
		}}
		r := NewCallerScopeResolver(fake)

		member, err := r.IsProjectMember(context.Background(), callerScopeTestProjectID, callerScopeTestEmail)
		if err != nil || !member {
			t.Fatalf("IsProjectMember = (%v, %v), want (true, nil)", member, err)
		}
	})

	t.Run("false when the matching contact does not grant case access", func(t *testing.T) {
		fake := &fakeEntityContacts{byProjectID: map[string][]entity.ProjectContact{
			callerScopeTestProjectID: {{Email: callerScopeTestEmail, GrantsCaseAccess: false}},
		}}
		r := NewCallerScopeResolver(fake)

		member, err := r.IsProjectMember(context.Background(), callerScopeTestProjectID, callerScopeTestEmail)
		if err != nil || member {
			t.Fatalf("IsProjectMember = (%v, %v), want (false, nil)", member, err)
		}
	})

	t.Run("false when no contact matches the email", func(t *testing.T) {
		fake := &fakeEntityContacts{byProjectID: map[string][]entity.ProjectContact{
			callerScopeTestProjectID: {{Email: "someone-else@example.com", GrantsCaseAccess: true}},
		}}
		r := NewCallerScopeResolver(fake)

		member, err := r.IsProjectMember(context.Background(), callerScopeTestProjectID, callerScopeTestEmail)
		if err != nil || member {
			t.Fatalf("IsProjectMember = (%v, %v), want (false, nil)", member, err)
		}
	})

	t.Run("propagates a SearchProjectContacts failure", func(t *testing.T) {
		wantErr := errors.New("upstream down")
		fake := &fakeEntityContacts{errByProjectID: map[string]error{callerScopeTestProjectID: wantErr}}
		r := NewCallerScopeResolver(fake)

		member, err := r.IsProjectMember(context.Background(), callerScopeTestProjectID, callerScopeTestEmail)
		if member || !errors.Is(err, wantErr) {
			t.Fatalf("IsProjectMember = (%v, %v), want (false, %v)", member, err, wantErr)
		}
	})

	t.Run("pages through the full contact list to find a later match", func(t *testing.T) {
		fake := &fakeEntityContacts{pagesByProjectID: map[string]map[int]entity.SearchProjectContactsResponse{
			callerScopeTestProjectID: {
				0: {Contacts: []entity.ProjectContact{{Email: "someone-else@example.com", GrantsCaseAccess: true}}, Total: 2},
				callerScopeContactsLimit: {
					Contacts: []entity.ProjectContact{{Email: callerScopeTestEmail, GrantsCaseAccess: true}}, Total: 2,
				},
			},
		}}
		r := NewCallerScopeResolver(fake)

		member, err := r.IsProjectMember(context.Background(), callerScopeTestProjectID, callerScopeTestEmail)
		if err != nil || !member {
			t.Fatalf("IsProjectMember = (%v, %v), want (true, nil)", member, err)
		}
	})
}

func TestProjectHandler_SearchProjects_CallerScope(t *testing.T) {
	memberProject := entity.ProjectView{ID: "member-project"}
	otherProject := entity.ProjectView{ID: "other-project"}
	errorProject := entity.ProjectView{ID: "error-project"}

	entityFake := &fakeEntityProjectResolver{
		searchResult: entity.SearchProjectsResponse{
			Projects: []entity.ProjectView{memberProject, otherProject, errorProject},
			Total:    3,
		},
	}
	contactsFake := &fakeEntityContacts{
		byProjectID: map[string][]entity.ProjectContact{
			"member-project": {{Email: callerScopeTestEmail, GrantsCaseAccess: true}},
			"other-project":  {{Email: "someone-else@example.com", GrantsCaseAccess: true}},
		},
		errByProjectID: map[string]error{
			"error-project": errors.New("upstream hiccup"),
		},
	}
	resolver := NewCallerScopeResolver(contactsFake)

	h := NewProjectHandler(entityFake)
	h.SetCallerScope(resolver, true)

	req := authedRequest(http.MethodPost, "/projects/search", `{"pagination":{"limit":10,"offset":0}}`)
	rec := httptest.NewRecorder()
	h.SearchProjects(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); !strings.Contains(got, "member-project") || strings.Contains(got, "other-project") || strings.Contains(got, "error-project") {
		t.Fatalf("expected only member-project in response, got %s", got)
	}
}

func TestProjectHandler_SearchProjects_CallerScopeDisabledByDefault(t *testing.T) {
	entityFake := &fakeEntityProjectResolver{
		searchResult: entity.SearchProjectsResponse{
			Projects: []entity.ProjectView{{ID: "any-project"}},
			Total:    1,
		},
	}
	h := NewProjectHandler(entityFake)
	// SetCallerScope deliberately not called.

	req := authedRequest(http.MethodPost, "/projects/search", `{"pagination":{"limit":10,"offset":0}}`)
	rec := httptest.NewRecorder()
	h.SearchProjects(rec, req)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "any-project") {
		t.Fatalf("expected unscoped 200 response including any-project, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCaseHandler_SearchCases_CallerScope(t *testing.T) {
	contactsFake := &fakeEntityContacts{
		byProjectID: map[string][]entity.ProjectContact{
			testProjectID: {{Email: callerScopeTestEmail, GrantsCaseAccess: true}},
		},
	}
	resolver := NewCallerScopeResolver(contactsFake)

	t.Run("member can search", func(t *testing.T) {
		fake := &fakeEntityCaseClient{}
		h := NewCaseHandler(fake)
		h.SetCallerScope(resolver, true)

		mux := http.NewServeMux()
		mux.HandleFunc("POST /projects/{id}/cases/search", h.SearchCases)
		req := authedRequest(http.MethodPost, "/projects/"+testProjectID+"/cases/search", `{}`)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("non-member is forbidden", func(t *testing.T) {
		fake := &fakeEntityCaseClient{}
		h := NewCaseHandler(fake)
		h.SetCallerScope(resolver, true)

		otherProjectID := "33333333-3333-3333-3333-333333333333"
		mux := http.NewServeMux()
		mux.HandleFunc("POST /projects/{id}/cases/search", h.SearchCases)
		req := authedRequest(http.MethodPost, "/projects/"+otherProjectID+"/cases/search", `{}`)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestCaseHandler_GetCase_CallerScope(t *testing.T) {
	contactsFake := &fakeEntityContacts{
		byProjectID: map[string][]entity.ProjectContact{
			testProjectID: {{Email: callerScopeTestEmail, GrantsCaseAccess: true}},
		},
	}
	resolver := NewCallerScopeResolver(contactsFake)

	t.Run("member can view the case", func(t *testing.T) {
		fake := &fakeEntityCaseClientForCase{caseView: entity.CaseView{ID: "case-1", ProjectDetails: entity.EntityRef{ID: testProjectID}}}
		h := NewCaseHandler(fake)
		h.SetCallerScope(resolver, true)

		mux := http.NewServeMux()
		mux.HandleFunc("GET /cases/{id}", h.GetCase)
		req := authedRequest(http.MethodGet, "/cases/44444444-4444-4444-4444-444444444444", "")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("non-member gets 404, not 403", func(t *testing.T) {
		otherProjectID := "55555555-5555-5555-5555-555555555555"
		fake := &fakeEntityCaseClientForCase{caseView: entity.CaseView{ID: "case-2", ProjectDetails: entity.EntityRef{ID: otherProjectID}}}
		h := NewCaseHandler(fake)
		h.SetCallerScope(resolver, true)

		mux := http.NewServeMux()
		mux.HandleFunc("GET /cases/{id}", h.GetCase)
		req := authedRequest(http.MethodGet, "/cases/44444444-4444-4444-4444-444444444444", "")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

// fakeEntityCaseClientForCase serves a fixed CaseView from GetCase; all
// other entityCaseClient methods are unused (nil-embedded).
type fakeEntityCaseClientForCase struct {
	entityCaseClient
	caseView entity.CaseView
}

func (f *fakeEntityCaseClientForCase) GetCase(_ context.Context, _ string) (entity.CaseView, error) {
	return f.caseView, nil
}
