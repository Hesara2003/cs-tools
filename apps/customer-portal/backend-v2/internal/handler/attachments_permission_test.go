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

	"github.com/wso2-open-operations/cs-tools/apps/customer-portal/backend-v2/internal/entity"
	"github.com/wso2-open-operations/cs-tools/apps/customer-portal/backend-v2/internal/middleware"
)

// fakeRoleResolver resolves every caller to a single fixed role, so each test
// case can exercise one point in the permission matrix.
type fakeRoleResolver struct {
	role middleware.CanonicalRole
	err  error
}

func (f fakeRoleResolver) GetRoles(ctx context.Context) ([]middleware.CanonicalRole, error) {
	if f.err != nil {
		return nil, f.err
	}
	return []middleware.CanonicalRole{f.role}, nil
}

// fakeAttachmentPermissionClient serves a fixed attachment (by referenceType)
// and records whether DeleteAttachment reached entity-service.
type fakeAttachmentPermissionClient struct {
	entityAttachmentClient
	referenceType *entity.ReferenceType
	deleted       bool
}

func (f *fakeAttachmentPermissionClient) GetAttachment(ctx context.Context, id string) (entity.AttachmentDetails, error) {
	return entity.AttachmentDetails{ID: id, ReferenceType: f.referenceType}, nil
}

func (f *fakeAttachmentPermissionClient) DeleteAttachment(ctx context.Context, id string) (entity.DeleteAttachmentResponse, error) {
	f.deleted = true
	return entity.DeleteAttachmentResponse{Message: "deleted"}, nil
}

func refType(rt entity.ReferenceType) *entity.ReferenceType { return &rt }

// TestDeleteAttachment_PermissionByReferenceType covers the permission check
// attachmentReferenceModule drives: DELETE /attachments/{id} is the one
// delete path for every attachment type, so its allowed roles depend on what
// the attachment actually references, resolved at request time rather than
// through a single static middleware.RequirePermission like every other
// route in this backend.
func TestDeleteAttachment_PermissionByReferenceType(t *testing.T) {
	tests := map[string]struct {
		referenceType *entity.ReferenceType
		role          middleware.CanonicalRole
		wantStatus    int
	}{
		"case attachment, admin allowed": {
			referenceType: refType(entity.ReferenceTypeCase),
			role:          middleware.RoleAdmin,
			wantStatus:    http.StatusOK,
		},
		"case attachment, customer_user forbidden": {
			// cases.delete is admin-only in the permission matrix.
			referenceType: refType(entity.ReferenceTypeCase),
			role:          middleware.RoleCustomerUser,
			wantStatus:    http.StatusForbidden,
		},
		"deployment attachment, customer_user allowed": {
			// deployments.delete grants every role, unlike every sibling module.
			referenceType: refType(entity.ReferenceTypeDeployment),
			role:          middleware.RoleCustomerUser,
			wantStatus:    http.StatusOK,
		},
		"nil referenceType (entity-service didn't report one), customer_user forbidden": {
			// Falls back to ModuleCases, the fail-closed default for an
			// absent or unmodeled reference type.
			referenceType: nil,
			role:          middleware.RoleCustomerUser,
			wantStatus:    http.StatusForbidden,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fake := &fakeAttachmentPermissionClient{referenceType: tc.referenceType}
			h := NewAttachmentHandler(fake, fakeRoleResolver{role: tc.role})

			mux := http.NewServeMux()
			mux.HandleFunc("DELETE /attachments/{id}", h.DeleteAttachment)

			req := authedRequest(http.MethodDelete, "/attachments/"+testAttachmentID, "")
			rec := httptest.NewRecorder()

			mux.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			wantDeleted := tc.wantStatus == http.StatusOK
			if fake.deleted != wantDeleted {
				t.Errorf("entity-service DeleteAttachment called = %v, want %v", fake.deleted, wantDeleted)
			}
		})
	}
}

// TestDeleteAttachment_RoleResolutionFailure covers RequirePermission's own
// failure mode (502) mirrored here since this route resolves roles by hand
// rather than through the RequirePermission middleware wrapper.
func TestDeleteAttachment_RoleResolutionFailure(t *testing.T) {
	fake := &fakeAttachmentPermissionClient{referenceType: refType(entity.ReferenceTypeCase)}
	h := NewAttachmentHandler(fake, fakeRoleResolver{err: errors.New("upstream down")})

	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /attachments/{id}", h.DeleteAttachment)

	req := authedRequest(http.MethodDelete, "/attachments/"+testAttachmentID, "")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
	if fake.deleted {
		t.Error("entity-service DeleteAttachment was called despite unresolved roles")
	}
}
