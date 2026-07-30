import { describe, expect, it } from "vitest";
import { getDeployState, markStarted, runningStacks } from "./deployStore";

// The store subscribes to window on import; these helpers emit the same events
// useLiveUpdates fans out from the WebSocket.
function emit(type: string, payload: Record<string, unknown>) {
  window.dispatchEvent(
    new CustomEvent("hivedock:deploy", { detail: { type, payload } }),
  );
}

describe("deployStore", () => {
  it("buffers output for a stack no component is watching", () => {
    // The point of the store: these events arrive while the Stacks view is
    // unmounted (user navigated to Home mid-operation). Nothing is rendering,
    // yet the output must still be there on return.
    emit("deploy:start", { stack: "media", action: "pull" });
    emit("deploy:line", { stack: "media", line: "Pulling jellyfin" });
    emit("deploy:line", { stack: "media", line: "Pull complete" });

    const st = getDeployState("local", "media");
    expect(st.phase).toBe("running");
    expect(st.action).toBe("pull");
    expect(st.lines).toEqual(["Pulling jellyfin", "Pull complete"]);

    emit("deploy:end", { stack: "media", ok: true });
    expect(getDeployState("local", "media").phase).toBe("ok");
    expect(getDeployState("local", "media").lines).toHaveLength(2);
  });

  it("keeps each stack's output separate", () => {
    emit("deploy:start", { stack: "a", action: "up" });
    emit("deploy:start", { stack: "b", action: "restart" });
    emit("deploy:line", { stack: "a", line: "for a" });
    emit("deploy:line", { stack: "b", line: "for b" });

    expect(getDeployState("local", "a").lines).toEqual(["for a"]);
    expect(getDeployState("local", "b").lines).toEqual(["for b"]);
    expect(getDeployState("local", "b").action).toBe("restart");
  });

  it("keeps a same-named stack on two hosts separate", () => {
    emit("deploy:start", { host: "local", stack: "web", action: "up" });
    emit("deploy:start", { host: "homelab", stack: "web", action: "pull" });
    emit("deploy:line", { host: "local", stack: "web", line: "local line" });
    emit("deploy:line", { host: "homelab", stack: "web", line: "remote line" });

    expect(getDeployState("local", "web").lines).toEqual(["local line"]);
    expect(getDeployState("homelab", "web").lines).toEqual(["remote line"]);
    expect(getDeployState("homelab", "web").action).toBe("pull");
  });

  it("records a failed operation with its error", () => {
    emit("deploy:start", { stack: "broken", action: "up" });
    emit("deploy:end", { stack: "broken", ok: false, error: "exit status 1" });

    const st = getDeployState("local", "broken");
    expect(st.phase).toBe("error");
    expect(st.error).toBe("exit status 1");
  });

  it("caps retained lines so a long pull can't grow without bound", () => {
    emit("deploy:start", { stack: "noisy", action: "pull" });
    for (let i = 0; i < 2500; i++) emit("deploy:line", { stack: "noisy", line: `l${i}` });

    const { lines } = getDeployState("local", "noisy");
    expect(lines).toHaveLength(2000);
    // The tail is what matters — the newest line must survive the trim.
    expect(lines[lines.length - 1]).toBe("l2499");
    expect(lines[0]).toBe("l500");
  });

  it("reports an unknown stack as idle without creating an entry", () => {
    const st = getDeployState("local", "never-touched");
    expect(st.phase).toBe("idle");
    expect(st.lines).toEqual([]);
    expect(runningStacks()).not.toContain("local/never-touched");
  });

  it("marks running on click, before the server's deploy:start arrives", () => {
    markStarted("local", "optimistic", "up");
    expect(getDeployState("local", "optimistic").phase).toBe("running");
    expect(runningStacks()).toContain("local/optimistic");
  });

  it("resets the buffer when a new operation starts on the same stack", () => {
    emit("deploy:start", { stack: "reused", action: "pull" });
    emit("deploy:line", { stack: "reused", line: "old run" });
    emit("deploy:end", { stack: "reused", ok: true });

    emit("deploy:start", { stack: "reused", action: "up" });
    const st = getDeployState("local", "reused");
    expect(st.lines).toEqual([]);
    expect(st.action).toBe("up");
    expect(st.error).toBeNull();
  });
});
