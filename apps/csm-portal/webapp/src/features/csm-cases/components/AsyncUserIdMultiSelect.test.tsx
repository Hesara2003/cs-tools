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

import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import "@testing-library/jest-dom/vitest";
import AsyncUserIdMultiSelect from "@features/csm-cases/components/AsyncUserIdMultiSelect";
import { useInfiniteUserSearch } from "@features/csm-cases/api/useUserSearch";

vi.mock("@features/csm-cases/api/useUserSearch", () => ({
  useInfiniteUserSearch: vi.fn(),
}));

const mockedUseInfiniteUserSearch = vi.mocked(useInfiniteUserSearch);

afterEach(() => {
  mockedUseInfiniteUserSearch.mockReset();
});

const NO_RESULTS = {
  users: [],
  isFetching: false,
  isFetchingNextPage: false,
  hasNextPage: false,
  isError: false,
  fetchNextPage: vi.fn(),
};

describe("AsyncUserIdMultiSelect — @me label", () => {
  it("renders a selected value equal to currentUserId as 'Me', not the raw UUID", () => {
    mockedUseInfiniteUserSearch.mockReturnValue(NO_RESULTS);

    render(
      <AsyncUserIdMultiSelect
        values={["11111111-1111-1111-1111-111111111111"]}
        onChange={vi.fn()}
        currentUserId="11111111-1111-1111-1111-111111111111"
      />,
    );

    expect(screen.getByText("Me")).toBeInTheDocument();
    expect(
      screen.queryByText("11111111-1111-1111-1111-111111111111"),
    ).not.toBeInTheDocument();
  });

  it("falls back to the raw UUID when the value doesn't match currentUserId and has no nameSeed", () => {
    mockedUseInfiniteUserSearch.mockReturnValue(NO_RESULTS);

    render(
      <AsyncUserIdMultiSelect
        values={["22222222-2222-2222-2222-222222222222"]}
        onChange={vi.fn()}
        currentUserId="11111111-1111-1111-1111-111111111111"
      />,
    );

    expect(screen.getByText("22222222-2222-2222-2222-222222222222")).toBeInTheDocument();
    expect(screen.queryByText("Me")).not.toBeInTheDocument();
  });

  it("prefers a nameSeed label over the raw UUID for a non-'me' selection", () => {
    mockedUseInfiniteUserSearch.mockReturnValue(NO_RESULTS);

    render(
      <AsyncUserIdMultiSelect
        values={["22222222-2222-2222-2222-222222222222"]}
        onChange={vi.fn()}
        nameSeed={new Map([["22222222-2222-2222-2222-222222222222", "Jane Doe"]])}
      />,
    );

    expect(screen.getByText("Jane Doe")).toBeInTheDocument();
  });
});
