import { Component, type ErrorInfo, type ReactNode } from "react";

// ErrorBoundary keeps one view's render crash from blanking the whole app.
// Resetting `resetKey` (e.g. the active view) clears a caught error so
// navigating away recovers without a full reload.
export default class ErrorBoundary extends Component<
  { children: ReactNode; resetKey?: unknown },
  { error: Error | null }
> {
  state = { error: null as Error | null };

  static getDerivedStateFromError(error: Error) {
    return { error };
  }

  componentDidUpdate(prev: { resetKey?: unknown }) {
    if (prev.resetKey !== this.props.resetKey && this.state.error) {
      this.setState({ error: null });
    }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("View crashed:", error, info.componentStack);
  }

  render() {
    if (this.state.error) {
      return (
        <div className="rounded-xl border border-red-500/30 bg-red-500/5 p-6 text-sm">
          <h2 className="mb-1 font-medium text-red-300">Something went wrong</h2>
          <p className="mb-3 text-zinc-400">
            This view hit an unexpected error. Try another tab, or reload.
          </p>
          <pre className="mb-3 overflow-x-auto rounded-md border border-zinc-800 bg-zinc-950 p-2.5 font-mono text-[11px] text-zinc-500">
            {this.state.error.message}
          </pre>
          <button
            onClick={() => window.location.reload()}
            className="rounded-lg border border-zinc-700 px-3 py-1.5 text-xs text-zinc-200 transition hover:bg-zinc-800"
          >
            Reload
          </button>
        </div>
      );
    }
    return this.props.children;
  }
}
