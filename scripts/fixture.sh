#!/usr/bin/env bash
# Create dev sample stacks that later features get tested against.
# Usage: scripts/fixture.sh [target-dir]   (default: ./dev-stacks)
set -euo pipefail

DIR="${1:-./dev-stacks}"
echo "Creating dev fixtures in: $DIR"
mkdir -p "$DIR"

write() {
  local path="$1"
  mkdir -p "$(dirname "$path")"
  cat >"$path"
  echo "  + ${path#"$DIR"/}"
}

# 1. whoami — trivial single published port (URL heuristic baseline)
write "$DIR/whoami/compose.yaml" <<'EOF'
services:
  whoami:
    image: traefik/whoami:v1.10.1
    ports:
      - "8081:80"
    restart: unless-stopped
EOF

# 2. nginx — plain static, latest tag (digest update path)
write "$DIR/nginx/compose.yaml" <<'EOF'
services:
  nginx:
    image: nginx:latest
    ports:
      - "8082:80"
    restart: unless-stopped
EOF

# 3. redis + app — two services, sidecar auto-hide heuristic
write "$DIR/redis-app/compose.yaml" <<'EOF'
services:
  app:
    image: traefik/whoami:v1.10.1
    ports:
      - "8083:80"
    depends_on:
      - redis
    restart: unless-stopped
  redis:
    image: redis:7.2-alpine
    restart: unless-stopped
EOF

# 4. homepage-labeled — exercises the homepage.* label fallback (migration path)
write "$DIR/homepage-labeled/compose.yaml" <<'EOF'
services:
  dashy:
    image: nginx:1.27-alpine
    ports:
      - "8084:80"
    restart: unless-stopped
    labels:
      homepage.name: "My Service"
      homepage.group: "Media"
      homepage.icon: "jellyfin.png"
EOF

# 5. env-tag — image tag via .env; must be surfaced as env-managed, never rewritten
write "$DIR/env-tag/compose.yaml" <<'EOF'
services:
  redis:
    image: redis:${REDIS_TAG}
    restart: unless-stopped
EOF
write "$DIR/env-tag/.env" <<'EOF'
REDIS_TAG=7.2-alpine
EOF

echo "Done. Point STACKS_DIR at $DIR (task dev does this automatically)."
