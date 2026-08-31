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

import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import "@testing-library/jest-dom/vitest";
import type { JSX } from "react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { UseQueryResult } from "@tanstack/react-query";
import type { BeIncidentDetail } from "@api/backend/types";

const navigateMock = vi.fn();
const useGetIncidentMock = vi.fn();
const patchMutateMock = vi.fn();
const showErrorMock = vi.fn();

// The backend client reads runtime config (`CSM_PORTAL_BACKEND_BASE_URL`) at
// module load, which isn't present under vitest. The page imports
// `BackendApiError` from it directly, so stub the module with a real class
// (so `instanceof` still works) — same approach as
// CsmChangeRequestDetailPage.test.tsx.
vi.mock("@api/backend/client", () => ({
  BackendApiError: class BackendApiError extends Error {
    status: number;
    constructor(status: number, message: string) {
      super(message);
      this.status = status;
    }
  },
  useBackendApi: () => ({ get: vi.fn(), patch: vi.fn() }),
}));

vi.mock("@hooks/useNavTransition", () => ({
  useNavTransition: () => navigateMock,
}));
vi.mock("@features/csm-operations/api/useGetIncident", () => ({
  useGetIncident: () => useGetIncidentMock(),
}));
vi.mock("@features/csm-operations/api/usePatchIncident", () => ({
  usePatchIncident: () => ({ isPending: false, mutate: patchMutateMock }),
}));
vi.mock("@context/error-banner/ErrorBannerContext", () => ({
  useErrorBanner: () => ({ showError: showErrorMock }),
}));
vi.mock("@features/csm-operations/api/useCsmIncidentComments", () => ({
  useGetCsmIncidentComments: () => ({ data: [] }),
  usePostCsmIncidentComment: () => ({ isPending: false, mutate: vi.fn() }),
}));
vi.mock("@features/csm-operations/api/useCsmIncidentActivities", () => ({
  useGetCsmIncidentActivities: () => ({ data: [] }),
}));
vi.mock("@features/csm-cases/api/useCsmCaseAttachments", () => ({
  useGetCsmCaseAttachments: () => ({ data: [] }),
  usePostCsmCaseAttachment: () => ({ isPending: false, mutate: vi.fn() }),
  useDownloadCsmCaseAttachment: () => vi.fn(),
  useGetCsmCaseAttachmentPreviewSource: () => vi.fn(),
}));
vi.mock("@features/csm-cases/components/CaseActivitiesFeed", () => ({
  default: () => null,
}));
vi.mock("@features/csm-cases/components/CaseDetailWidgets", () => ({
  AttachmentsWidget: () => null,
}));
vi.mock("@api/useSearchUsersByName", () => ({
  useSearchUsersByName: () => ({ data: [], isFetching: false, isError: false }),
}));

// Imported after the mocks above so the module picks them up.
import CsmIncidentDetailPage from "@features/csm-operations/pages/CsmIncidentDetailPage";

const BASE_INCIDENT: BeIncidentDetail = {
  id: "inc-1",
  number: "INC0012345",
  openedOn: "2026-01-01T00:00:00Z",
  subject: "Gateway 502s",
  priority: null,
  state: "IN_PROGRESS" as BeIncidentDetail["state"],
  category: null,
  parent: { id: "inc-parent", name: "INC0011111" },
  changeRequest: { id: "chg-1", name: "CHG0009988" },
  problem: { id: "prb-1", name: "PRB0040157" },
  causedBy: { id: "obscure-1", name: "Some obscure record" },
};

function mockQueryResult(
  overrides: Partial<UseQueryResult<BeIncidentDetail | null, Error>>,
): void {
  useGetIncidentMock.mockReturnValue({
    data: null,
    isLoading: false,
    isError: false,
    error: null,
    ...overrides,
  });
}

function clickChip(text: string): void {
  screen
    .getByText(text)
    .closest('[role="button"]')
    ?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
}

function goToTab(name: RegExp): void {
  fireEvent.click(screen.getByRole("tab", { name }));
}

/** Surfaces the router's current search string, for the `?tab=` sync tests
 * below. */
function LocationSearchProbe(): JSX.Element {
  const location = useLocation();
  return <div data-testid="search-probe">{location.search}</div>;
}

/**
 * Real `<MemoryRouter>`/`<Routes>` (not a mocked `react-router`), matching
 * this app's own convention for a hook/page that reads the router itself
 * (see `useNormalizedIdParam.test.tsx`) — `useQueryParamTabs` needs a real
 * `useSearchParams` to actually read/write the URL, which a mocked
 * `react-router` module can't provide.
 */
function renderPage(initialEntry = "/operations/incidents/inc-1"): ReturnType<typeof render> {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialEntry]}>
        <Routes>
          <Route
            path="/operations/incidents/:id"
            element={
              <>
                <CsmIncidentDetailPage />
                <LocationSearchProbe />
              </>
            }
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

// `patchMutateMock`/`showErrorMock`/`navigateMock` are module-scoped `vi.fn()`s
// shared across every test in this file — without a reset, a call recorded by
// an earlier test (e.g. the direct-PATCH transition test) is still present
// when a later test asserts `not.toHaveBeenCalled()`, failing on someone else's
// call instead of its own.
afterEach(() => {
  vi.clearAllMocks();
});

describe("CsmIncidentDetailPage — tabs", () => {
  it("renders all five tabs and defaults to Activities", () => {
    mockQueryResult({ data: BASE_INCIDENT });
    renderPage();

    expect(screen.getByRole("tab", { name: /activities/i })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(screen.getByRole("tab", { name: /details/i })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /related/i })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /watchers/i })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /attachments/i })).toBeInTheDocument();
  });

  it("switches to the Details tab and shows classification fields", () => {
    mockQueryResult({ data: { ...BASE_INCIDENT, category: "INQUIRY" } });
    renderPage();
    goToTab(/details/i);
    expect(screen.getByText("Classification")).toBeInTheDocument();
    expect(screen.getByText("INQUIRY")).toBeInTheDocument();
  });

  it("renders parent incident, change request, and problem as clickable references under Related", () => {
    mockQueryResult({ data: BASE_INCIDENT });
    renderPage();
    goToTab(/related/i);

    clickChip("INC0011111");
    expect(navigateMock).toHaveBeenCalledWith("/operations/incidents/inc-parent");

    clickChip("CHG0009988");
    expect(navigateMock).toHaveBeenCalledWith("/operations/change-requests/chg-1");

    clickChip("PRB0040157");
    expect(navigateMock).toHaveBeenCalledWith("/operations/problems/prb-1");
  });

  it("renders 'Caused by' as plain, non-navigable text since its target type is unconfirmed", () => {
    mockQueryResult({ data: BASE_INCIDENT });
    renderPage();
    goToTab(/related/i);

    const causedByText = screen.getByText("Some obscure record");
    expect(causedByText.closest('[role="button"]')).toBeNull();
  });

  it("shows the watch list under the Watchers tab", () => {
    mockQueryResult({
      data: {
        ...BASE_INCIDENT,
        watchList: [{ id: "u1", name: "Jane Doe", email: "jane.doe@example.com" }],
      },
    });
    renderPage();
    goToTab(/watchers/i);
    expect(screen.getByText("Jane Doe")).toBeInTheDocument();
  });

  it("does not render a Comments & notes card on the Details tab (duplicates the Activities tab)", () => {
    mockQueryResult({
      data: {
        ...BASE_INCIDENT,
        additionalComments: "Customer says the issue recurred.",
        workNotes: "Checked the gateway logs.",
      },
    });
    renderPage();
    goToTab(/details/i);
    expect(screen.queryByText("Comments & notes")).not.toBeInTheDocument();
    expect(screen.queryByText("Customer says the issue recurred.")).not.toBeInTheDocument();
    expect(screen.queryByText("Checked the gateway logs.")).not.toBeInTheDocument();
  });
});

describe("CsmIncidentDetailPage — Watchers tab is read-only (watchList PATCH is a confirmed-live 404)", () => {
  it("renders watcher chips with no remove affordance", () => {
    mockQueryResult({
      data: {
        ...BASE_INCIDENT,
        watchList: [{ id: "u1", name: "Jane Doe", email: "jane.doe@example.com" }],
      },
    });
    renderPage();
    goToTab(/watchers/i);

    const chip = screen.getByText("Jane Doe").closest(".MuiChip-root");
    expect(chip?.querySelector(".MuiChip-deleteIcon")).toBeNull();
  });

  it("disables the 'Add watcher' button", () => {
    mockQueryResult({ data: { ...BASE_INCIDENT, watchList: [] } });
    renderPage();
    goToTab(/watchers/i);

    expect(screen.getByRole("button", { name: /add watcher/i })).toBeDisabled();
    expect(patchMutateMock).not.toHaveBeenCalled();
  });
});

describe("CsmIncidentDetailPage — tab lives in the URL", () => {
  it("writes the selected tab to ?tab= when switching tabs", () => {
    mockQueryResult({ data: BASE_INCIDENT });
    renderPage();

    goToTab(/watchers/i);

    expect(screen.getByTestId("search-probe")).toHaveTextContent("tab=watchers");
  });

  it("restores the tab named in the URL on a direct/cold load, instead of always defaulting to Activities", () => {
    mockQueryResult({ data: BASE_INCIDENT });
    renderPage("/operations/incidents/inc-1?tab=details");

    expect(screen.getByRole("tab", { name: /details/i })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  it("falls back to Activities for an unrecognised ?tab= value, without crashing", () => {
    mockQueryResult({ data: BASE_INCIDENT });
    renderPage("/operations/incidents/inc-1?tab=not-a-real-tab");

    expect(screen.getByRole("tab", { name: /activities/i })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });
});

describe("CsmIncidentDetailPage — state-transition action bar", () => {
  // IN_PROGRESS has three legal next states (ON_HOLD, RESOLVED, CANCELLED),
  // so IncidentActionBar renders a "Change state" menu rather than a single
  // button — open it first, same interaction as CaseActionBar's multi-target
  // case.
  function openChangeState(): void {
    fireEvent.click(screen.getByRole("button", { name: /change state/i }));
  }

  it("dispatches a direct PATCH for a simple transition (IN_PROGRESS -> ON_HOLD)", () => {
    mockQueryResult({ data: { ...BASE_INCIDENT, state: "IN_PROGRESS" } });
    renderPage();
    openChangeState();
    fireEvent.click(screen.getByRole("menuitem", { name: /on hold/i }));
    expect(patchMutateMock).toHaveBeenCalledWith(
      { id: "inc-1", patch: { state: "ON_HOLD" } },
      expect.objectContaining({ onError: expect.any(Function) }),
    );
  });

  it("opens the resolution dialog for RESOLVED instead of PATCHing immediately", () => {
    mockQueryResult({ data: { ...BASE_INCIDENT, state: "IN_PROGRESS" } });
    renderPage();
    openChangeState();
    fireEvent.click(screen.getByRole("menuitem", { name: /resolved/i }));
    expect(patchMutateMock).not.toHaveBeenCalled();
    // The dialog title and its submit button both read "Move to Resolved" —
    // scope to the heading so this doesn't trip over the multiple-match error
    // a plain getByText(/move to resolved/i) would throw.
    expect(screen.getByRole("heading", { name: /move to resolved/i })).toBeInTheDocument();
  });

  it("submits the resolution dialog with state + resolutionCode + resolutionNotes", () => {
    mockQueryResult({ data: { ...BASE_INCIDENT, state: "IN_PROGRESS" } });
    renderPage();
    openChangeState();
    fireEvent.click(screen.getByRole("menuitem", { name: /resolved/i }));

    fireEvent.change(screen.getByLabelText(/resolution code/i), {
      target: { value: "Solved" },
    });
    fireEvent.change(screen.getByLabelText(/resolution notes/i), {
      target: { value: "Restarted the service." },
    });
    fireEvent.click(screen.getByRole("button", { name: /^move to resolved$/i }));

    expect(patchMutateMock).toHaveBeenCalledWith(
      {
        id: "inc-1",
        patch: {
          state: "RESOLVED",
          resolutionCode: "Solved",
          resolutionNotes: "Restarted the service.",
        },
      },
      expect.objectContaining({
        onSuccess: expect.any(Function),
        onError: expect.any(Function),
      }),
    );
  });

  it("renders no state-transition buttons for a terminal incident (CLOSED)", () => {
    mockQueryResult({ data: { ...BASE_INCIDENT, state: "CLOSED" } });
    renderPage();
    expect(screen.queryByRole("button", { name: /in progress/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /change state/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /cancelled/i })).not.toBeInTheDocument();
  });

  it("renders no state-transition buttons for a terminal incident (CANCELLED)", () => {
    mockQueryResult({ data: { ...BASE_INCIDENT, state: "CANCELLED" } });
    renderPage();
    expect(screen.queryByRole("button", { name: /in progress/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /change state/i })).not.toBeInTheDocument();
  });
});
