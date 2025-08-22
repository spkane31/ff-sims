# Multi-stage Dockerfile with nginx reverse proxy

# Stage 1: Build Go backend
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

# Stage 2: Build Next.js frontend
FROM node:18-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm install --production=false --max_old_space_size=2048
COPY frontend/ ./
RUN NODE_OPTIONS="--max_old_space_size=2048" npm run build

# Stage 3: Final runtime image with nginx
FROM nginx:alpine
RUN apk --no-cache add ca-certificates nodejs npm supervisor

# Copy nginx configuration
COPY nginx.conf /etc/nginx/nginx.conf

# Copy Go backend binary
COPY --from=backend-builder /app/backend/main /app/backend/

# Copy Next.js application
COPY --from=frontend-builder /app/frontend/.next /app/frontend/.next
COPY --from=frontend-builder /app/frontend/public /app/frontend/public
COPY --from=frontend-builder /app/frontend/package*.json /app/frontend/
COPY --from=frontend-builder /app/frontend/node_modules /app/frontend/node_modules

# Create supervisor configuration
RUN echo '[supervisord]' > /etc/supervisord.conf && \
    echo 'nodaemon=true' >> /etc/supervisord.conf && \
    echo 'user=root' >> /etc/supervisord.conf && \
    echo 'silent=false' >> /etc/supervisord.conf && \
    echo '' >> /etc/supervisord.conf && \
    echo '[program:nginx]' >> /etc/supervisord.conf && \
    echo 'command=nginx -g "daemon off;"' >> /etc/supervisord.conf && \
    echo 'priority=100' >> /etc/supervisord.conf && \
    echo 'autostart=true' >> /etc/supervisord.conf && \
    echo 'autorestart=true' >> /etc/supervisord.conf && \
    echo 'stdout_logfile=/dev/stdout' >> /etc/supervisord.conf && \
    echo 'stdout_logfile_maxbytes=0' >> /etc/supervisord.conf && \
    echo 'stderr_logfile=/dev/stderr' >> /etc/supervisord.conf && \
    echo 'stderr_logfile_maxbytes=0' >> /etc/supervisord.conf && \
    echo 'stdout_events_enabled=true' >> /etc/supervisord.conf && \
    echo 'stderr_events_enabled=true' >> /etc/supervisord.conf && \
    echo '' >> /etc/supervisord.conf && \
    echo '[program:backend]' >> /etc/supervisord.conf && \
    echo 'command=/app/backend/main' >> /etc/supervisord.conf && \
    echo 'directory=/app' >> /etc/supervisord.conf && \
    echo 'environment=SERVER_PORT=8080' >> /etc/supervisord.conf && \
    echo 'priority=200' >> /etc/supervisord.conf && \
    echo 'autostart=true' >> /etc/supervisord.conf && \
    echo 'autorestart=true' >> /etc/supervisord.conf && \
    echo 'stdout_logfile=/dev/stdout' >> /etc/supervisord.conf && \
    echo 'stdout_logfile_maxbytes=0' >> /etc/supervisord.conf && \
    echo 'stderr_logfile=/dev/stderr' >> /etc/supervisord.conf && \
    echo 'stderr_logfile_maxbytes=0' >> /etc/supervisord.conf && \
    echo 'stdout_events_enabled=true' >> /etc/supervisord.conf && \
    echo 'stderr_events_enabled=true' >> /etc/supervisord.conf && \
    echo '' >> /etc/supervisord.conf && \
    echo '[program:frontend]' >> /etc/supervisord.conf && \
    echo 'command=npm start' >> /etc/supervisord.conf && \
    echo 'directory=/app/frontend' >> /etc/supervisord.conf && \
    echo 'environment=PORT=3000' >> /etc/supervisord.conf && \
    echo 'priority=300' >> /etc/supervisord.conf && \
    echo 'autostart=true' >> /etc/supervisord.conf && \
    echo 'autorestart=true' >> /etc/supervisord.conf && \
    echo 'stdout_logfile=/dev/stdout' >> /etc/supervisord.conf && \
    echo 'stdout_logfile_maxbytes=0' >> /etc/supervisord.conf && \
    echo 'stderr_logfile=/dev/stderr' >> /etc/supervisord.conf && \
    echo 'stderr_logfile_maxbytes=0' >> /etc/supervisord.conf && \
    echo 'stdout_events_enabled=true' >> /etc/supervisord.conf && \
    echo 'stderr_events_enabled=true' >> /etc/supervisord.conf

# Install supervisor
RUN apk add --no-cache supervisor

# Expose only port 80 (nginx)
EXPOSE 80

# Start supervisor to manage all processes
CMD ["supervisord", "-c", "/etc/supervisord.conf"]