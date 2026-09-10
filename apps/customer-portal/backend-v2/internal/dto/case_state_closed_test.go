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

import "testing"

// TestIsCaseStateClosed pins the three representations a case state reaches
// this helper as — entity-service's domain enum, ServiceNow's display text,
// and the numeric SN choice-list id the frontend still speaks — and, just as
// importantly, that no other state is mistaken for closed. The closed-case
// attachment guards in internal/handler decide whether to reject a write on
// this answer alone, so a false positive silently blocks a customer from
// attaching a file to a live case.
func TestIsCaseStateClosed(t *testing.T) {
	closed := map[string]string{
		"domain enum":           "closed",
		"ServiceNow label":      "Closed",
		"uppercase":             "CLOSED",
		"padded":                "  closed  ",
		"ServiceNow numeric id": "3",
	}
	for name, state := range closed {
		if !IsCaseStateClosed(state) {
			t.Errorf("%s: IsCaseStateClosed(%q) = false, want true", name, state)
		}
	}

	open := map[string]string{
		"empty":                        "",
		"open":                         "open",
		"work in progress enum":        "work_in_progress",
		"work in progress label":       "Work In Progress",
		"solution proposed":            "solution_proposed",
		"reopened":                     "reopened",
		"open's numeric id":            "1",
		"unknown state":                "on_hold",
		"substring of a closed label":  "close",
		"closed-looking longer string": "closed_pending_review",
	}
	for name, state := range open {
		if IsCaseStateClosed(state) {
			t.Errorf("%s: IsCaseStateClosed(%q) = true, want false", name, state)
		}
	}
}
