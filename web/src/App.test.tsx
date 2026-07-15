import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, test, vi } from "vitest";
import App from "./App";

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

function mockApi() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes("/api/host/stats")) {
        return jsonResponse({
          available: false,
          cpuPercent: 0,
          memUsedBytes: 0,
          memTotalBytes: 0,
          numCpu: 0,
        });
      }
      if (url.includes("/api/home")) {
        return jsonResponse([
          {
            stack: "jellyfin",
            service: "jellyfin",
            name: "Jellyfin",
            group: "Media",
            url: "http://host:8096",
            iconSlug: "jellyfin",
            status: "running",
            hidden: false,
          },
        ]);
      }
      if (url.includes("/api/stacks")) {
        return jsonResponse([
          {
            name: "whoami",
            project: "whoami",
            origin: "managed",
            status: "running",
            composeFile: "/srv/stacks/whoami/compose.yaml",
            services: [
              {
                name: "whoami",
                image: "traefik/whoami:v1.10.1",
                state: "running",
                ports: [{ public: 8081, private: 80, type: "tcp" }],
              },
            ],
          },
        ]);
      }
      if (url.includes("/api/updates")) {
        return jsonResponse([]);
      }
      if (url.includes("/api/auth/status")) {
        return jsonResponse({
          needsSetup: false,
          authenticated: true,
          username: "admin",
        });
      }
      if (url.includes("/api/app/update")) {
        return jsonResponse({
          current: "1.2.3",
          hasUpdate: false,
          checkable: true,
        });
      }
      if (url.includes("/api/health")) {
        return jsonResponse({
          status: "ok",
          version: "test",
          stacksDir: "/srv/stacks",
          time: "2026-07-10T00:00:00Z",
        });
      }
      throw new Error(`unexpected url ${url}`);
    }),
  );
}

function renderApp() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <App />
    </QueryClientProvider>,
  );
}

afterEach(() => {
  vi.restoreAllMocks();
});

test("renders the home dashboard by default", async () => {
  mockApi();
  renderApp();

  expect(screen.getByText("hivedock")).toBeInTheDocument();
  // Cards render in a single flat grid (no per-group headers); the card shows
  // the app name and its stack as the subtitle.
  await waitFor(() => expect(screen.getByText("Jellyfin")).toBeInTheDocument());
  expect(screen.getByText("jellyfin")).toBeInTheDocument();
});

test("navigating to Stacks shows the stacks list", async () => {
  mockApi();
  renderApp();

  fireEvent.click(screen.getByRole("button", { name: /Stacks/i }));
  await waitFor(() => expect(screen.getByText("whoami")).toBeInTheDocument());
  expect(screen.getByText("Managed")).toBeInTheDocument();
});

test("sidebar shows backend health and version", async () => {
  mockApi();
  renderApp();

  // The old Status page moved into the sidebar footer; the version is its
  // own always-visible line (turns into an update pill when one exists).
  await waitFor(() =>
    expect(screen.getByText(/Backend ok/)).toBeInTheDocument(),
  );
  await waitFor(() => expect(screen.getByText("v1.2.3")).toBeInTheDocument());
});
