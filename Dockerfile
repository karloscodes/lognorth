# Build stage
FROM golang:1.26-alpine AS builder

# Install build dependencies (build-base includes gcc, g++, make)
RUN apk add --no-cache build-base musl-dev git nodejs npm

WORKDIR /app

# Cache Go dependencies (invalidates only when go.mod/go.sum changes)
COPY go.mod go.sum ./
RUN go mod download

# Cache npm dependencies (invalidates only when package.json changes)
COPY web/package*.json web/package-lock.json ./web/
RUN cd web && npm ci --legacy-peer-deps --production=false

# Copy web source and build frontend first (needed for go:embed)
COPY web/src ./web/src
COPY web/public ./web/public
COPY web/vite.config.ts web/tsconfig*.json web/postcss.config.js web/tailwind.config.ts ./web/
RUN cd web && npm run build

# Copy Go source files (including embed.go at root)
COPY embed.go ./
COPY cmd ./cmd
COPY internal ./internal

# Build Go binaries (frontend assets are embedded via go:embed)
RUN mkdir -p dist && \
  CGO_ENABLED=1 go build -o dist/lognorth-server ./cmd/lognorth/ && \
  CGO_ENABLED=1 go build -o dist/lnctl ./cmd/lnctl/ && \
  CGO_ENABLED=1 go build -o dist/seed-demo ./cmd/seed-demo/

# Final stage - minimal runtime image
FROM alpine:3.19

# Link GHCR package to public releases repo
LABEL org.opencontainers.image.source="https://github.com/karloscodes/lognorth-releases"

WORKDIR /app

# Install runtime dependencies and create directories
RUN apk add --no-cache ca-certificates tzdata sqlite curl && \
  mkdir -p /app/logs /app/storage

# Copy compiled binaries from builder (assets are embedded in the binary)
COPY --from=builder /app/dist/lognorth-server /app/lognorth-server
COPY --from=builder /app/dist/lnctl /app/lnctl
COPY --from=builder /app/dist/seed-demo /app/seed-demo

# Copy entrypoint script
COPY scripts/entrypoint.sh /app/entrypoint.sh

# Set permissions in one RUN command
RUN chmod +x /app/entrypoint.sh /app/lognorth-server /app/lnctl /app/seed-demo

ENV LOGNORTH_ENV=production \
  LOGNORTH_STORAGE_PATH=/app/storage \
  LOGNORTH_PORT=8080

EXPOSE 8080

# Health check using curl
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD curl -f http://localhost:8080/_health || exit 1

ENTRYPOINT ["/app/entrypoint.sh"]
