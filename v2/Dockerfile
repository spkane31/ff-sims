# Multi-stage Dockerfile serving from Go backend only

# Stage 1: Build Next.js frontend
FROM node:18-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm install --production=false --max_old_space_size=2048
COPY frontend/ ./
RUN NODE_OPTIONS="--max_old_space_size=2048" npm run build

# Stage 2: Build Go backend
FROM golang:1.24-alpine AS backend-builder
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

# Stage 3: Final runtime image with Go serving everything
FROM alpine:latest
RUN apk --no-cache add ca-certificates curl

# Copy Go backend binary
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

# Start the Go server
CMD ["/app/backend/main"]