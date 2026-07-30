# Multi-host — end-to-end test scenario

This is the manual acceptance run for Phase 2 (full remote stack management). Each
step maps to a check; sign each off before tagging a `v1.3.0-rc`. Automated tests
(`go test ./...`, `go vet`, `gofmt`, web `tsc`/`eslint`/`vitest`, plus in-process
manager+agent e2e) already cover the transport and backend — this exercises the
whole path against real Docker.

## Two ways to get a "remote" host

- **Real:** run the agent on your home server, pointed at the VPS/prod manager.
- **Self-contained (one machine):** run a second `hivedock agent` container against
  a *separate* `STACKS_DIR`, so it shows up as a distinct host next to `local`.

The commands below use the self-contained setup; swap the manager URL / stacks
path for the real case.

## 0. Prerequisites

- The agent host has the Docker socket **and** the compose plugin
  (`docker compose version` succeeds).
- The agent's `--stacks-dir` is bind-mounted at the **same path** the compose
  files resolve against (a mismatch → the agent runs read-only; that is checked in
  step 7).

## 1. Enable + enroll

1. Manager → **Settings → Hosts → Add a host**. Confirm a token is shown once and a
   `docker run … hivedock agent …` command appears.
2. Run the command on the second host (set `--name homelab`, a real `--stacks-dir`).
   Self-contained example:

   ```
   docker run -d --name hivedock-agent --restart unless-stopped \
     -v /var/run/docker.sock:/var/run/docker.sock \
     -v /opt/stacks-remote:/stacks \
     ghcr.io/rodzalendo/hivedock:latest \
     agent --manager http://<manager-host>:5002 \
           --token <token> --name homelab --stacks-dir /stacks
   ```

3. **Check:** the **host switcher** appears at the top of the Stacks page and
   `homelab` shows **online** (Settings/Hosts and `/api/hosts` also list it).

## 2. See remote stacks

Switch the host switcher to `homelab`. **Check:** the list shows the remote host's
stacks/containers (its `STACKS_DIR`), not the manager's.

## 3. Create + deploy on the remote host

1. **New** → create a stack (blank, or **From docker run**) while `homelab` is
   selected → edit its compose → **Deploy**.
2. **Check:** live output **streams** in the Operation pane, and the containers come
   up **on the remote host** — verify with `docker ps` there.

## 4. Logs + exec on the remote host

1. Open the remote stack's **Logs** (streaming, follow toggles).
2. Open a running service's **shell** and run `hostname` / `ls`.
3. **Check:** the shell is inside the **remote** container (its hostname/filesystem),
   and logs stream from it.

## 5. Edit + update

1. Change an image tag via the compose editor (or the per-stack update apply) →
   **Save** → **Deploy**.
2. **Check:** the new image runs on the remote host; the optimistic-lock 409 flow
   still works (edit the file out of band, then save → reconcile prompt).

## 6. Restart + delete

1. Per-service **restart** (spinner clears on completion).
2. **Delete** the test stack.
3. **Check:** the service restarts and the stack is gone on the remote host.

## 7. Isolation + failure checks

- **Same-named stacks don't collide:** create a stack named `web` on both `local`
  and `homelab`; deploy output and logs stay separated; switching back to `local`
  shows the manager's own `web`.
- **Read-only agent refuses writes:** start an agent whose `--stacks-dir` is
  bind-mismatched (mount a different host path than the container path). **Check:**
  reads work, but a create/deploy/edit fails with a clear read-only error.
- **Offline is clean:** stop the agent container. **Check:** `homelab` flips to
  **offline** in the switcher, and its actions fail cleanly ("host is offline"),
  never hang; `local` is unaffected. Restart the agent → it reconnects and shows
  online again.
