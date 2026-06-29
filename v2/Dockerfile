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

# Accept build arguments
ARG GIT_SHA=unknown
ARG BUILD_TIME=unknown

COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -o main \
    -ldflags="-X 'backend/pkg/version.GitSHA=${GIT_SHA}' -X 'backend/pkg/version.BuildTime=${BUILD_TIME}'" \
    ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -o worker ./cmd/worker

# Stage 3: Build Python ESPN worker
# Pin to bookworm so the compiled Python and .venv extensions share glibc 2.36
# with the runtime stage. The untagged python:3.12-slim moved to Trixie (glibc 2.38)
# which is incompatible with debian:bookworm-slim.
FROM python:3.12-slim-bookworm AS espn-worker-builder
WORKDIR /app/workers/espn
RUN pip install --no-cache-dir uv
COPY workers/espn/pyproject.toml workers/espn/uv.lock ./
RUN uv sync --frozen --no-dev
COPY workers/espn/ ./

# Stage 4: Final runtime image with Go serving everything
# python:3.12-slim-bookworm provides the Python runtime that matches the builder,
# eliminating glibc version mismatches. Go binaries are CGO_ENABLED=0 (statically
# linked) and work on any Linux. We skip copying Python itself from the builder
# because the runtime image already carries the identical interpreter and stdlib.
FROM python:3.12-slim-bookworm
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl openssl \
    && rm -rf /var/lib/apt/lists/*

# Copy Go backend binaries (statically linked — no libc dependency)
COPY --from=backend-builder /app/backend/main /app/backend/
COPY --from=backend-builder /app/backend/worker /app/backend/

# Copy Python ESPN worker, its virtualenv, and uv
# Python interpreter + stdlib are already present in the base image.
COPY --from=espn-worker-builder /app/workers/espn /app/workers/espn
COPY --from=espn-worker-builder /usr/local/bin/uv /usr/local/bin/uv

# Entrypoint: Go Sleeper worker + Python ESPN worker + HTTP server
# --no-sync: venv was frozen at build time; prevents uv from attempting network
# access inside the container if the lockfile appears out-of-date.
RUN printf '#!/bin/sh\n/app/backend/worker &\ncd /app/workers/espn && uv run --no-sync python worker.py &\nexec /app/backend/main\n' > /entrypoint.sh && chmod +x /entrypoint.sh

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

# Start worker (background) + HTTP server (foreground)
CMD ["/entrypoint.sh"]