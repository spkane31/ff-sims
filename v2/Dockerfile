# Multi-stage Dockerfile for Go backend + React frontend

# Stage 1: Build React frontend
FROM --platform=linux/amd64 node:18-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
# Build Next.js with static export
RUN npm run build

# Stage 2: Build Go backend
FROM golang:1.24-alpine AS backend-builder
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
# Create static directory and copy the output files
RUN mkdir -p ./static
# Copy the Next.js static output files
COPY --from=frontend-builder /app/frontend/out ./static
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

# Stage 3: Final runtime image
FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/

# Copy the Go binary
COPY --from=backend-builder /app/backend/main .
# Copy the static files
COPY --from=backend-builder /app/backend/static ./static

# Expose port
EXPOSE 8080

# Run the binary
CMD ["./main"]