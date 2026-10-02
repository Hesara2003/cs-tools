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

import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi, beforeEach } from "vitest";
import type { ReactNode } from "react";
import type { User } from "@features/csm-users/types/csmUsers";

const postMock = vi.fn();

// The real client reads runtime config at module load, which isn't present
// under vitest (same approach as useDeployedProductOptions.test.tsx).
vi.mock("@api/backend/client", () => ({
  useBackendApi: () => ({ post: postMock }),
}));

import { useInfiniteUserSearch } from "@features/csm-cases/api/useUserSearch";

function wrapper({ children }: { children: ReactNode }) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

function makeUser(overrides: Partial<User>): User {
  return {
    id: "u-1",
    userName: "jane.doe",
    firstName: "Jane",
    lastName: "Doe",
    email: "jane.doe@wso2.com",
    userType: "internal",
    ...overrides,
  };
}

describe("useInfiniteUserSearch", () => {
  beforeEach(() => {
    postMock.mockReset();
  });

  // Regression test: entity-service's "user" table has no uniqueness
  // constraint on email, so two real rows can carry the same address with
  // different case/whitespace. Reported live as duplicate-looking entries in
  // the "Created by" dropdown.
  it("de-duplicates users whose email differs only by case or whitespace", async () => {
    postMock.mockResolvedValue({
      users: [
        makeUser({ id: "u-1", email: "jane.doe@wso2.com" }),
        makeUser({ id: "u-2", email: "Jane.Doe@wso2.com " }),
        makeUser({ id: "u-3", userName: "john.smith", firstName: "John", lastName: "Smith", email: "john.smith@wso2.com" }),
      ],
      total: 3,
      limit: 10,
      offset: 0,
      hasMore: false,
    });

    const { result } = renderHook(() => useInfiniteUserSearch("", true), { wrapper });

    await waitFor(() => expect(result.current.isFetching).toBe(false));

    expect(result.current.users).toHaveLength(2);
    expect(result.current.users.map((u) => u.email)).toEqual([
      "jane.doe@wso2.com",
      "john.smith@wso2.com",
    ]);
  });

  it("keeps distinct users with genuinely different emails", async () => {
    postMock.mockResolvedValue({
      users: [
        makeUser({ id: "u-1", email: "jane.doe@wso2.com" }),
        makeUser({ id: "u-2", userName: "john.smith", firstName: "John", lastName: "Smith", email: "john.smith@wso2.com" }),
      ],
      total: 2,
      limit: 10,
      offset: 0,
      hasMore: false,
    });

    const { result } = renderHook(() => useInfiniteUserSearch("", true), { wrapper });

    await waitFor(() => expect(result.current.isFetching).toBe(false));

    expect(result.current.users).toHaveLength(2);
  });
});
