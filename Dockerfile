# Build stage
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache build-base musl-dev git nodejs npm

WORKDIR /app

# Cache Go dependencies
COPY go.mod go.sum ./
RUN go mod download

# Cache npm dependencies
COPY web/package*.json web/package-lock.json ./web/
RUN cd web && npm ci --legacy-peer-deps --production=false

# Copy source files
COPY Makefile ./
COPY cmd ./cmd
COPY internal ./internal

# Copy web source
COPY web/src ./web/src
COPY web/public ./web/public
COPY web/index.html web/vite.config.ts web/tsconfig*.json web/postcss.config.js web/tailwind.config.ts ./web/

# Build binaries and web assets
RUN mkdir -p dist && \
  CGO_ENABLED=1 go build -o dist/logbuddy-server cmd/logbuddy/main.go && \
  CGO_ENABLED=1 go build -o dist/lbctl cmd/lbctl/main.go && \
  cd web && npm run build

# Final stage
FROM alpine:3.19
WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata sqlite curl && \
  mkdir -p /app/logs /app/storage /app/web/dist

COPY --from=builder /app/dist/logbuddy-server /app/logbuddy-server
COPY --from=builder /app/dist/lbctl /app/lbctl
COPY --from=builder /app/web/dist /app/web/dist

COPY scripts/entrypoint.sh /app/entrypoint.sh

RUN chmod +x /app/entrypoint.sh /app/logbuddy-server /app/lbctl

ENV LOGBUDDY_ENV=production \
  LOGBUDDY_STORAGE_PATH=/app/storage \
  LOGBUDDY_PORT=8080

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD curl -f http://localhost:8080/_health || exit 1

ENTRYPOINT ["/app/entrypoint.sh"]
