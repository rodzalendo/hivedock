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
      if (url.includes("/api/health")) {
        return jsonResponse({
          status: "ok",
          version: "test",
          stacksDir: "/srv/stacks",
          authDisabled: true,
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

test("renders the stacks list by default", async () => {
  mockApi();
  renderApp();

  expect(screen.getByText("Hivedock")).toBeInTheDocument();
  await waitFor(() => expect(screen.getByText("whoami")).toBeInTheDocument());
  expect(screen.getByText("Managed")).toBeInTheDocument();
});

test("navigating to Status shows backend health", async () => {
  mockApi();
  renderApp();

  fireEvent.click(screen.getByRole("button", { name: /Status/i }));
  await waitFor(() => expect(screen.getByText("Health: ok")).toBeInTheDocument());
});
