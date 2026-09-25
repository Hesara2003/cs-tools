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

// entityAttachmentClient abstracts the entity-service attachment operations
// used by AttachmentHandler.
type entityAttachmentClient interface {
	CreateAttachment(ctx context.Context, req entity.CreateAttachmentRequest) (entity.CreateAttachmentResponse, error)
	SearchAttachments(ctx context.Context, req entity.SearchAttachmentsRequest) (entity.SearchAttachmentsResponse, error)
	GetAttachmentContent(ctx context.Context, id string) (body []byte, contentType string, err error)
	DeleteAttachment(ctx context.Context, id string) (entity.DeleteAttachmentResponse, error)
	GetAttachment(ctx context.Context, id string) (entity.AttachmentDetails, error)
	// GetCase backs DeleteAttachment's closed-case guard only — this handler
	// serves no case route of its own (see caseIsClosed in cases.go).
	GetCase(ctx context.Context, id string) (entity.CaseView, error)
}

// AttachmentHandler handles HTTP requests for attachment operations.
type AttachmentHandler struct {
	entity entityAttachmentClient
	roles  middleware.RoleResolver
}

// NewAttachmentHandler creates an AttachmentHandler backed by the given entity
// client and role resolver. The resolver is needed because DeleteAttachment
// (see its doc comment) is the one delete path for every attachment type and
// has to pick its permission module at request time, after the attachment's
// own referenceType is known -- it can't be wrapped with a single static
// middleware.RequirePermission the way every other route in this backend is.
func NewAttachmentHandler(entity entityAttachmentClient, roles middleware.RoleResolver) *AttachmentHandler {
	return &AttachmentHandler{entity: entity, roles: roles}
}

// attachmentReferenceModule maps an attachment's referenceType to the
// permission-matrix module that governs deleting it. nil (entity-service
// doesn't always report a reference type -- see AttachmentDetails' doc
// comment), change_request, and incident attachments have no dedicated
// module in the matrix (this backend has no change_request/incident
// attachment routes today) all fall back to ModuleCases -- delete is
// admin-only there, the strictest module available, which is the correct
// fail-closed default when the reference type is absent or unmodeled.
func attachmentReferenceModule(refType *entity.ReferenceType) middleware.Module {
	if refType != nil && *refType == entity.ReferenceTypeDeployment {
		return middleware.ModuleDeployments
	}
	return middleware.ModuleCases
}

// CreateAttachment handles POST /attachments.
func (h *AttachmentHandler) CreateAttachment(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserInfoFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, ErrMsgUnauthorized)
		return
	}

	body, ok := readJSONBody(w, r)
	if !ok {
		return
	}

	var req entity.CreateAttachmentRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}

	result, err := h.entity.CreateAttachment(r.Context(), req)
	if err != nil {
		slog.ErrorContext(r.Context(), "entity CreateAttachment failed", "userID", user.UserID, "err", summarizeErr(err))
		mapUpstreamError(w, err, "Failed to create attachment.")
		return
	}

	writeJSONValue(w, http.StatusCreated, dto.MapAttachmentCreate(result))
}

// SearchAttachments handles POST /attachments/search.
func (h *AttachmentHandler) SearchAttachments(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserInfoFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, ErrMsgUnauthorized)
		return
	}

	body, ok := readJSONBody(w, r)
	if !ok {
		return
	}

	var req entity.SearchAttachmentsRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}

	result, err := h.entity.SearchAttachments(r.Context(), req)
	if err != nil {
		slog.ErrorContext(r.Context(), "entity SearchAttachments failed", "userID", user.UserID, "err", summarizeErr(err))
		mapUpstreamError(w, err, "Failed to search attachments.")
		return
	}

	writeJSONValue(w, http.StatusOK, dto.MapSearchAttachments(result))
}

// GetAttachmentContent handles GET /attachments/{id}/content. The response is
// the raw file content, not JSON. Content-Disposition: attachment is always
// set (mirroring entity-service's own XSS mitigation for this endpoint) so
// browsers never render an attachment inline.
func (h *AttachmentHandler) GetAttachmentContent(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserInfoFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, ErrMsgUnauthorized)
		return
	}

	id := r.PathValue("id")
	if id == "" || !isAttachmentID(id) {
		writeError(w, http.StatusBadRequest, ErrMsgInvalidUUID)
		return
	}

	content, contentType, err := h.entity.GetAttachmentContent(r.Context(), id)
	if err != nil {
		slog.ErrorContext(r.Context(), "entity GetAttachmentContent failed", "userID", user.UserID, "attachmentID", id, "err", summarizeErr(err))
		mapUpstreamError(w, err, "Failed to download attachment.")
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", "attachment")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content) // #nosec G705 -- Content-Type set from entity-service's own sanitized value; Content-Disposition forces download, never inline rendering
}

// DeleteAttachment handles DELETE /attachments/{id}. Rejected with 400 when the
// attachment belongs to a closed case (see caseIsClosed).
func (h *AttachmentHandler) DeleteAttachment(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserInfoFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, ErrMsgUnauthorized)
		return
	}

	id := r.PathValue("id")
	if id == "" || !isAttachmentID(id) {
		writeError(w, http.StatusBadRequest, ErrMsgInvalidUUID)
		return
	}

	// This route is not nested under a case (or deployment, or conversation),
	// so the attachment has to be fetched first to learn what it actually
	// references -- both for the closed-case rule below and, now, for
	// authorization: this is the only delete path for every attachment type,
	// so it cannot be wrapped with a single static middleware.RequirePermission
	// the way every other route in this backend is. A lookup failure fails
	// closed (403), not open -- unlike the closed-case check below, which
	// fails open by design (see caseIsClosed's doc comment).
	attachment, err := h.entity.GetAttachment(r.Context(), id)
	if err != nil {
		slog.ErrorContext(r.Context(), "entity GetAttachment failed during DeleteAttachment", "userID", user.UserID, "attachmentID", id, "err", summarizeErr(err))
		mapUpstreamError(w, err, "Failed to delete attachment.")
		return
	}

	roles, err := h.roles.GetRoles(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "rbac: failed to resolve roles for DeleteAttachment", "userID", user.UserID, "err", summarizeErr(err))
		writeError(w, http.StatusBadGateway, "Failed to resolve user roles.")
		return
	}
	if !middleware.HasPermission(roles, attachmentReferenceModule(attachment.ReferenceType), middleware.ActionDelete) {
		writeError(w, http.StatusForbidden, ErrMsgForbidden)
		return
	}

	if attachment.ReferenceID != "" && caseIsClosed(r.Context(), h.entity, attachment.ReferenceID) {
		slog.WarnContext(r.Context(), "rejected attachment delete on a closed case", "userID", user.UserID, "attachmentID", id, "caseID", attachment.ReferenceID)
		writeError(w, http.StatusBadRequest, ErrMsgCaseClosedForAttachmentDelete)
		return
	}

	result, err := h.entity.DeleteAttachment(r.Context(), id)
	if err != nil {
		slog.ErrorContext(r.Context(), "entity DeleteAttachment failed", "userID", user.UserID, "attachmentID", id, "err", summarizeErr(err))
		mapUpstreamError(w, err, "Failed to delete attachment.")
		return
	}

	writeJSONValue(w, http.StatusOK, dto.MapDeleteAttachment(result))
}

// GetAttachment handles GET /attachments/{id} — metadata plus base64-encoded
// content, distinct from GetAttachmentContent's raw binary stream.
func (h *AttachmentHandler) GetAttachment(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserInfoFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, ErrMsgUnauthorized)
		return
	}

	id := r.PathValue("id")
	if id == "" || !isAttachmentID(id) {
		writeError(w, http.StatusBadRequest, ErrMsgInvalidUUID)
		return
	}

	result, err := h.entity.GetAttachment(r.Context(), id)
	if err != nil {
		slog.ErrorContext(r.Context(), "entity GetAttachment failed", "userID", user.UserID, "attachmentID", id, "err", summarizeErr(err))
		mapUpstreamError(w, err, "Failed to retrieve attachment.")
		return
	}

	writeJSONValue(w, http.StatusOK, dto.MapAttachmentDetails(result))
}
