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
	"strings"
)

// CallerScopeResolver answers "is this email a portal-user contact of this
// project". There is no bulk "which projects does this email belong to"
// lookup anywhere in this stack (entity-service has no caller-identity
// filter on project search, and the project-contact onboarding service
// only exposes contacts-for-a-project, never projects-for-a-contact) — so
// every scoping decision here is answered one project at a time, by
// resolving the project's Salesforce ID and checking its contact list,
// rather than via a reverse index.
type CallerScopeResolver struct {
	entity   entityProjectResolver
	contacts contactsClient
}

// NewCallerScopeResolver constructs a CallerScopeResolver backed by the same
// entity and project-contact onboarding service clients ContactHandler uses.
func NewCallerScopeResolver(entityClient entityProjectResolver, contactsClient contactsClient) *CallerScopeResolver {
	return &CallerScopeResolver{entity: entityClient, contacts: contactsClient}
}

// IsProjectMember reports whether email belongs to projectID as an active
// portal-user contact. A non-nil error means the check itself failed
// (upstream unavailable, unknown project, ...) — callers should treat that
// the same as "not a member" (fail closed) rather than let an upstream
// hiccup silently grant access.
func (r *CallerScopeResolver) IsProjectMember(ctx context.Context, projectID, email string) (bool, error) {
	project, err := r.entity.GetProject(ctx, projectID)
	if err != nil {
		return false, err
	}

	contacts, err := r.contacts.GetProjectContacts(ctx, project.SfID)
	if err != nil {
		return false, err
	}

	for _, c := range contacts {
		if c.IsPortalUser && strings.EqualFold(c.Email, email) {
			return true, nil
		}
	}
	return false, nil
}
