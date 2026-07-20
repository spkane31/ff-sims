# Multi-stage Dockerfile serving from Go backend only

# Stage 1: Build Next.js frontend
FROM node:18-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm install --production=false --max_old_space_size=2048
COPY frontend/ ./
RUN NODE_OPTIONS="--max_old_space_size=2048" npm run build

# Stage 2: Build Go backend
FROM golang:1.25-alpine AS backend-builder
WORKDIR /app/backend

# Accept an optional GIT_SHA override (e.g. for a CI system that already knows
# the SHA). When not passed as a build-arg, it's computed below from the .git
# directory in the build context so the image always carries a real per-commit
# ID for cmd/server's /api/health version info -- previously this silently
# defaulted to the literal string "unknown" whenever the build invocation
# omitted --build-arg GIT_SHA=..., which is what DigitalOcean's App Platform
# build does. Back when this Dockerfile also built cmd/worker, that same gap
# broke Worker Deployment Versioning (every DO worker registered under build ID
# "unknown"); now that cmd/worker is only built on the worker host, this GIT_SHA
# is purely for API version reporting.
ARG GIT_SHA=""
ARG BUILD_TIME=unknown

COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY .git /tmp/git-meta
# cmd/worker is NOT built here: DigitalOcean no longer runs a Temporal worker.
# The worker host (deploy/worker-host/{deploy,setup}.sh) is the sole fleet that
# builds and runs cmd/worker, and the sole promoter of the shared Worker
# Deployment — see docs/worker-versioning.md.
RUN apk add --no-cache git \
    && GIT_SHA="${GIT_SHA:-$(git --git-dir=/tmp/git-meta rev-parse --short=9 HEAD)}" \
    && rm -rf /tmp/git-meta \
    && CGO_ENABLED=0 GOOS=linux go build -o main \
       -ldflags="-X 'backend/pkg/version.GitSHA=${GIT_SHA}' -X 'backend/pkg/version.BuildTime=${BUILD_TIME}'" \
       ./cmd/server

# Stage 3: Final runtime image
# The Python ESPN worker no longer runs here — it moved to the worker host
# (deploy/worker-host/) as a native systemd service alongside the Go Temporal
# worker, for direct `journalctl` access instead of digging through container
# logs. See deploy/worker-host/README.md.
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*

# Copy Go backend binary (statically linked — no libc dependency)
COPY --from=backend-builder /app/backend/main /app/backend/

# Copy Next.js build output
COPY --from=frontend-builder /app/frontend/.next /app/frontend/.next
COPY --from=frontend-builder /app/frontend/public /app/frontend/public

# Create a simple healthcheck script
RUN echo '#!/bin/sh' > /healthcheck.sh && \
    echo 'echo "=== Health Check Starting ==="' >> /healthcheck.sh && \
    echo 'echo "Checking backend and frontend via Go server..."' >> /healthcheck.sh && \
    echo 'curl -f http://localhost:8080/api/health || exit 1' >> /healthcheck.sh && \
    echo 'curl -f http://localhost:8080/ || exit 1' >> /healthcheck.sh && \
    echo 'echo "=== Health Check Complete ==="' >> /healthcheck.sh && \
    chmod +x /healthcheck.sh

# Expose port 8080 (Go server)
EXPOSE 8080

# Add healthcheck
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
  CMD /healthcheck.sh

# Start the HTTP server
CMD ["/app/backend/main"]