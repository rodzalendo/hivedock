import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, test, vi } from "vitest";
import App from "./App";

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

test("renders health status from the API", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      new Response(
        JSON.stringify({
          status: "ok",
          version: "test",
          stacksDir: "/opt/stacks-test",
          authDisabled: true,
          time: "2026-07-10T00:00:00Z",
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    ),
  );

  renderApp();

  expect(screen.getByText("Hivedock")).toBeInTheDocument();
  await waitFor(() =>
    expect(screen.getByText("Health: ok")).toBeInTheDocument(),
  );
  expect(screen.getByText("/opt/stacks-test")).toBeInTheDocument();
});
