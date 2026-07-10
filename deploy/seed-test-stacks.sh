#!/usr/bin/env bash
# Seed the 7 test stacks into /opt/stacks-test on PCT 102 (see docs/DEPLOYMENT.md).
# Chosen to exercise each subsystem: URL heuristic, semver, digest path,
# part-count preservation, sidecar hide, homepage.* fallback, env-managed tags.
#
# Idempotent: only writes files; never runs `docker compose up`. Hivedock (or a
# human) brings them up. Never point this at the real /opt/stacks.
set -euo pipefail

DIR="${STACKS_TEST_DIR:-/opt/stacks-test}"
echo "Seeding test stacks into: $DIR"
mkdir -p "$DIR"

write() {
  local path="$1"
  mkdir -p "$(dirname "$path")"
  cat >"$path"
  echo "  + ${path#"$DIR"/}"
}

# 1. whoami — trivial, one published port → URL heuristic baseline
write "$DIR/whoami/compose.yaml" <<'EOF'
services:
  whoami:
    image: traefik/whoami:v1.10.1
    ports:
      - "8081:80"
    restart: unless-stopped
EOF

# 2. jellyfin — pinned OLD tag → semver update path + icon match
write "$DIR/jellyfin/compose.yaml" <<'EOF'
services:
  jellyfin:
    image: jellyfin/jellyfin:10.10.3
    ports:
      - "8096:8096"
    restart: unless-stopped
EOF

# 3. uptime-kuma — :1 major-pin tag → part-count preservation test
write "$DIR/uptime-kuma/compose.yaml" <<'EOF'
services:
  uptime-kuma:
    image: louislam/uptime-kuma:1
    ports:
      - "3001:3001"
    restart: unless-stopped
EOF

# 4. nginx-latest — latest tag → digest update path
write "$DIR/nginx-latest/compose.yaml" <<'EOF'
services:
  nginx:
    image: nginx:latest
    ports:
      - "8082:80"
    restart: unless-stopped
EOF

# 5. app-with-db — web app + postgres → sidecar auto-hide heuristic
write "$DIR/app-with-db/compose.yaml" <<'EOF'
services:
  app:
    image: traefik/whoami:v1.10.1
    ports:
      - "8083:80"
    depends_on:
      - db
    restart: unless-stopped
  db:
    image: postgres:16.4
    environment:
      POSTGRES_PASSWORD: example
    restart: unless-stopped
EOF

# 6. homepage-labeled — homepage.* labels → migration fallback
write "$DIR/homepage-labeled/compose.yaml" <<'EOF'
services:
  web:
    image: nginx:1.27-alpine
    ports:
      - "8084:80"
    restart: unless-stopped
    labels:
      homepage.name: "Labeled Service"
      homepage.group: "Tools"
      homepage.icon: "nginx.png"
EOF

# 7. env-tag — image: redis:${REDIS_TAG} → must be surfaced env-managed, never rewritten
write "$DIR/env-tag/compose.yaml" <<'EOF'
services:
  redis:
    image: redis:${REDIS_TAG}
    restart: unless-stopped
EOF
write "$DIR/env-tag/.env" <<'EOF'
REDIS_TAG=7.2-alpine
EOF

echo "Done. $DIR seeded with 7 test stacks (not started)."
