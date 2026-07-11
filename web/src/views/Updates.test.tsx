import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, test, vi } from "vitest";
import Updates from "./Updates";

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function renderUpdates() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <Updates />
    </QueryClientProvider>,
  );
}

afterEach(() => vi.restoreAllMocks());

test("shows an available semver update with candidate and diff", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      if (String(input).includes("/api/updates")) {
        return jsonResponse([
          {
            image: "traefik/whoami:v1.10.0",
            kind: "semver",
            hasUpdate: true,
            current: "v1.10.0",
            candidate: "v1.11.0",
            diff: "minor",
            usedBy: [{ stack: "media", service: "whoami" }],
          },
        ]);
      }
      throw new Error(`unexpected ${String(input)}`);
    }),
  );

  renderUpdates();
  await waitFor(() =>
    expect(screen.getByText("traefik/whoami:v1.10.0")).toBeInTheDocument(),
  );
  expect(screen.getByText("v1.11.0")).toBeInTheDocument();
  expect(screen.getByText("minor")).toBeInTheDocument();
  expect(screen.getByText("1 update available")).toBeInTheDocument();
});

test("Check now posts to the check endpoint", async () => {
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (url.includes("/api/updates/check")) {
      return jsonResponse({ images: 2 });
    }
    if (url.includes("/api/updates")) {
      return jsonResponse([]);
    }
    throw new Error(`unexpected ${url} ${init?.method ?? ""}`);
  });
  vi.stubGlobal("fetch", fetchMock);

  renderUpdates();
  await waitFor(() =>
    expect(screen.getByText("Everything up to date")).toBeInTheDocument(),
  );

  fireEvent.click(screen.getByRole("button", { name: /Check now/i }));

  await waitFor(() =>
    expect(
      fetchMock.mock.calls.some(
        ([u, i]) =>
          String(u).includes("/api/updates/check") &&
          (i as RequestInit | undefined)?.method === "POST",
      ),
    ).toBe(true),
  );
});
