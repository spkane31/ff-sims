# Multi-stage Dockerfile for Go backend + Next.js frontend

# Stage 1: Build Go backend
FROM golang:1.24-alpine AS backend-builder
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -o main ./cmd/server

# Stage 2: Build Next.js frontend
FROM node:18-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
# Increase memory limit and use npm install instead of npm ci for better memory management
RUN npm install --production=false --max_old_space_size=2048
COPY frontend/ ./
# Build Next.js for production with memory optimization
RUN NODE_OPTIONS="--max_old_space_size=2048" npm run build

# Stage 3: Final runtime image with both servers
FROM node:18-alpine
RUN apk --no-cache add ca-certificates
WORKDIR /app

# Copy Go backend binary
COPY --from=backend-builder /app/backend/main ./backend/

# Copy Next.js application
COPY --from=frontend-builder /app/frontend/.next ./frontend/.next
COPY --from=frontend-builder /app/frontend/public ./frontend/public
COPY --from=frontend-builder /app/frontend/package*.json ./frontend/
COPY --from=frontend-builder /app/frontend/node_modules ./frontend/node_modules

# Create startup script
RUN echo '#!/bin/sh' > start.sh && \
    echo 'cd /app/frontend && npm start &' >> start.sh && \
    echo 'cd /app && ./backend/main &' >> start.sh && \
    echo 'wait' >> start.sh && \
    chmod +x start.sh

# Expose ports for both services
EXPOSE 3000 8080

# Start both services
CMD ["./start.sh"]