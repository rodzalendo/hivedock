import { useEffect } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { fetchAuthStatus } from "./api";
import AuthScreen from "./views/AuthScreen";

// AuthGate decides what the user sees before the app: a loading state, the
// first-run setup form, the login form, or (when authenticated or auth is
// disabled) the app itself. It re-checks on a `hivedock:unauthorized` event so
// an expired session drops back to the login screen.
export default function AuthGate({ children }: { children: React.ReactNode }) {
  const qc = useQueryClient();
  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: ["auth-status"],
    queryFn: fetchAuthStatus,
    retry: false,
    staleTime: 60_000,
  });

  useEffect(() => {
    const handler = () => void refetch();
    window.addEventListener("hivedock:unauthorized", handler);
    return () => window.removeEventListener("hivedock:unauthorized", handler);
  }, [refetch]);

  const onAuthed = () => {
    // Drop any data cached under an unauthenticated session, then re-check.
    void qc.invalidateQueries();
    void refetch();
  };

  if (isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-zinc-950 text-sm text-zinc-500">
        Loading…
      </div>
    );
  }

  if (isError || !data) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-zinc-950 text-sm text-red-400">
        Could not reach the server.
      </div>
    );
  }

  if (!data.authenticated) {
    return (
      <AuthScreen mode={data.needsSetup ? "setup" : "login"} onDone={onAuthed} />
    );
  }

  return <>{children}</>;
}
