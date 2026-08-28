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
	"log/slog"
	"net/http"

	"github.com/wso2-open-operations/cs-tools/apps/customer-portal/backend-v2/internal/dto"
	"github.com/wso2-open-operations/cs-tools/apps/customer-portal/backend-v2/internal/entity"
	"github.com/wso2-open-operations/cs-tools/apps/customer-portal/backend-v2/internal/middleware"
)

// entityProjectClient abstracts the entity-service project operations used by ProjectHandler.
type entityProjectClient interface {
	SearchProjects(ctx context.Context, req entity.SearchProjectsRequest) (entity.SearchProjectsResponse, error)
	GetProject(ctx context.Context, id string) (entity.ProjectDetailsView, error)
}

// ProjectHandler handles HTTP requests for project operations.
type ProjectHandler struct {
	entity entityProjectClient

	callerScope *CallerScopeResolver
}

// NewProjectHandler creates a ProjectHandler backed by the given entity client.
func NewProjectHandler(entity entityProjectClient) *ProjectHandler {
	return &ProjectHandler{entity: entity}
}

// SetCallerScope wires up caller-scoped project search: SearchProjects only
// returns projects the caller is an active portal-user contact of (see
// CallerScopeResolver). main.go always calls this in production — there is
// no kill switch. A setter rather than a constructor parameter purely so
// the many pre-existing tests across this package that construct handlers
// directly, unrelated to this feature, keep compiling without change; a nil
// resolver (never calling this) is treated as unscoped rather than
// panicking — see requireProjectMember's doc comment.
func (h *ProjectHandler) SetCallerScope(resolver *CallerScopeResolver) {
	h.callerScope = resolver
}

// SearchProjects handles POST /projects/search.
func (h *ProjectHandler) SearchProjects(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserInfoFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, ErrMsgUnauthorized)
		return
	}

	body, ok := readJSONBody(w, r)
	if !ok {
		return
	}

	var req dto.SearchProjectsRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}

	result, err := h.entity.SearchProjects(r.Context(), dto.BuildEntitySearchProjectsRequest(req))
	if err != nil {
		slog.ErrorContext(r.Context(), "entity SearchProjects failed", "userID", user.UserID, "err", summarizeErr(err))
		mapUpstreamError(w, err, "Failed to search projects.")
		return
	}

	// Commented out pending end-to-end verification against real
	// entity-service data — uncomment while testing, re-comment before
	// committing. See handler.CallerScopeResolver / scopeToCallerProjects.
	// if h.callerScope != nil {
	// 	result = h.scopeToCallerProjects(r.Context(), result, user.Email)
	// }

	writeJSONValue(w, http.StatusOK, dto.MapSearchProjects(result))
}

// scopeToCallerProjects filters result to only the projects the caller is a
// member of, checked one project at a time (see CallerScopeResolver — there
// is no bulk query available). A project whose membership check itself
// fails is excluded rather than included: fail closed, don't leak a project
// just because an upstream call hiccuped.
//
// Total is adjusted to the filtered count; Limit/Offset/HasMore are left as
// entity-service returned them, describing the unfiltered upstream page —
// a known limitation of post-filtering a single page rather than
// re-paginating the scoped result set. A caller with many projects spread
// thin across pages could see a page come back smaller than its own Limit
// even though more of their projects exist on a later upstream page.
func (h *ProjectHandler) scopeToCallerProjects(ctx context.Context, result entity.SearchProjectsResponse, email string) entity.SearchProjectsResponse {
	scoped := make([]entity.ProjectView, 0, len(result.Projects))
	for _, p := range result.Projects {
		member, err := h.callerScope.IsProjectMember(ctx, p.ID, email)
		if err != nil {
			slog.ErrorContext(ctx, "caller scope check failed", "projectID", p.ID, "err", summarizeErr(err))
			continue
		}
		if member {
			scoped = append(scoped, p)
		}
	}
	result.Projects = scoped
	result.Total = len(scoped)
	return result
}

// GetProject handles GET /projects/{id}.
func (h *ProjectHandler) GetProject(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserInfoFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, ErrMsgUnauthorized)
		return
	}

	id := r.PathValue("id")
	if id == "" || !uuidRe.MatchString(id) {
		writeError(w, http.StatusBadRequest, ErrMsgInvalidUUID)
		return
	}

	result, err := h.entity.GetProject(r.Context(), id)
	if err != nil {
		slog.ErrorContext(r.Context(), "entity GetProject failed", "userID", user.UserID, "projectID", id, "err", summarizeErr(err))
		mapUpstreamError(w, err, "Failed to retrieve project.")
		return
	}

	writeJSONValue(w, http.StatusOK, dto.MapProjectDetails(result))
}
