# syntax=docker/dockerfile:1

# --- Stage 1: build the frontend -------------------------------------------
FROM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json* ./
RUN npm ci || npm install
COPY web/ ./
RUN npm run build

# --- Stage 2: build the Go binary ------------------------------------------
FROM golang:1.23-alpine AS build
WORKDIR /src
# The compose CLI is a runtime dependency (subprocess), not a build one.
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
# Bring in the freshly built SPA so go:embed has real assets.
COPY --from=web /web/dist ./web/dist
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -X github.com/rogalinski/hivedock/internal/server.version=${VERSION}" \
    -o /out/hivedock ./cmd/hivedock

# --- cosign: the signature-verification tool for the self-update check (§3.2).
# Pinned by digest — the tool that verifies our supply chain is itself verified.
FROM ghcr.io/sigstore/cosign/cosign:v2.4.3@sha256:c77247c92f4dfea851c70555738226498393e34e2f9ca83cb959e51c230e4ad7 AS cosign

# --- Stage 3: runtime -------------------------------------------------------
FROM alpine:3.20
# docker-cli + compose plugin: Hivedock shells out to `docker compose`.
# git: optional local audit trail of stack changes (opt-in, §5.4).
RUN apk add --no-cache docker-cli docker-cli-compose git ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/hivedock /usr/local/bin/hivedock
# cosign is a static Go binary — runs on alpine/musl as-is.
COPY --from=cosign /ko-app/cosign /usr/local/bin/cosign

ENV PORT=5001 \
    DATA_DIR=/app/data \
    STACKS_DIR=/opt/stacks \
    HOME=/app
# Seed the sigstore trust root into the image so self-update verification is
# offline: the only outbound call at check time is fetching the signature from
# ghcr (keeps the outbound inventory honest — §3.5). Best-effort: if the TUF CDN
# is unreachable at build, cosign fetches the root on first verify instead.
RUN cosign initialize || echo "cosign initialize skipped; trust root fetched at first verify"
EXPOSE 5001
VOLUME ["/app/data"]

# Run as root by default: the Docker socket is typically root-owned, and
# compose subprocess operations need it. Harden with socket-proxy later.
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:5001/api/health || exit 1

ENTRYPOINT ["/usr/local/bin/hivedock"]
