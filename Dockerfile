# syntax=docker/dockerfile:1.6

# Build stage for the frontend
FROM node:20 AS frontend-build
WORKDIR /app/webui
RUN corepack enable
COPY webui/pnpm-lock.yaml webui/package.json ./
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm fetch --frozen-lockfile
COPY webui/ ./
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm install --offline --frozen-lockfile
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm run build

# Build stage for the backend
FROM golang:1.22 AS backend-build
WORKDIR /app
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOPROXY=https://goproxy.io,direct go mod download
COPY . .
# Copy the built frontend from frontend build stage
COPY --from=frontend-build /app/webui/dist ./webui/dist
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o llmio .

# Final stage
FROM alpine:latest

WORKDIR /app

# Copy the binary from backend build stage
COPY --from=backend-build /app/llmio .

EXPOSE 7070

# Command to run the application
CMD ["./llmio"]
