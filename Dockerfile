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
FROM golang:1.23 AS backend-build
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
    go mod tidy
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o llmio .

# Final stage
FROM alpine:latest

# 创建非 root 用户
RUN adduser -D -u 1000 app

WORKDIR /app

# Copy the binary from backend build stage
COPY --from=backend-build /app/llmio .

# 创建数据库目录并设置权限
RUN mkdir -p /app/db && chown -R app:app /app

# 切换到非 root 用户
USER app

EXPOSE 7070

# 健康检查
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:7070/ || exit 1

# Command to run the application
CMD ["./llmio"]
