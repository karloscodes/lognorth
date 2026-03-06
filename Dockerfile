# Build stage
FROM golang:1.25-alpine AS builder

# Install build dependencies (build-base includes gcc, g++, make)
RUN apk add --no-cache build-base musl-dev git nodejs npm

WORKDIR /app

# Cache Go dependencies (invalidates only when go.mod/go.sum changes)
COPY go.mod go.sum ./
RUN go mod download

# Cache npm dependencies (invalidates only when package.json changes)
COPY web/package*.json web/package-lock.json ./web/
RUN cd web && npm ci --legacy-peer-deps --production=false

# Copy only necessary source files (excluding web/node_modules)
COPY Makefile ./
COPY cmd ./cmd
COPY internal ./internal

# Copy web source
COPY web/src ./web/src
COPY web/public ./web/public
COPY web/index.html web/vite.config.ts web/tsconfig*.json web/postcss.config.js web/tailwind.config.ts ./web/

# Build binaries and web assets
RUN mkdir -p dist && \
  CGO_ENABLED=1 go build -o dist/lognorth-server cmd/lognorth/main.go && \
  CGO_ENABLED=1 go build -o dist/lnctl cmd/lnctl/main.go && \
  CGO_ENABLED=1 go build -o dist/seed-demo cmd/seed-demo/main.go && \
  cd web && npm run build

# Final stage - minimal runtime image
FROM alpine:3.19

# Link GHCR package to public releases repo
LABEL org.opencontainers.image.source="https://github.com/karloscodes/lognorth-releases"

WORKDIR /app

# Install runtime dependencies, create app user, and create directories in one layer
RUN apk add --no-cache ca-certificates tzdata sqlite curl su-exec && \
  addgroup -S lognorth && \
  adduser -S -G lognorth -u 10001 -h /app lognorth && \
  mkdir -p /app/logs /app/storage /app/web/dist

# Copy compiled binaries and built assets from builder
COPY --from=builder /app/dist/lognorth-server /app/lognorth-server
COPY --from=builder /app/dist/lnctl /app/lnctl
COPY --from=builder /app/dist/seed-demo /app/seed-demo
COPY --from=builder /app/web/dist /app/web/dist

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
