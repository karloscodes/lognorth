# LogNorth

Logs that alert. Logs you read. Self-hosted error tracking in one container
and one SQLite file. Your data never leaves your server.

- Groups errors into issues and alerts on new ones, regressions, and spikes,
  measured against the baseline of your own traffic.
- Pings your app every minute and alerts after three failures in a row.
- Sends alerts by email, ntfy, Telegram, or webhook, with quiet hours.
- Lets a coding agent read production logs over MCP, read-only.

[Website](https://lognorth.com)

## Install

On your server, as root:

```bash
curl -fsSL lognorth.com/install | sudo bash -s logs.example.com
```

It installs LogNorth with [Chasen](https://chasenhq.com): HTTPS, live backups
of the database, and an update each night. Run it again to update now.

Or run the image yourself: `karloscodes/lognorth` on Docker Hub, or
`ghcr.io/karloscodes/lognorth-releases`.

## Send events

Use an SDK: [TypeScript](https://github.com/karloscodes/lognorth-sdk-ts),
[Go](https://github.com/karloscodes/lognorth-sdk-go), or
[Rails](https://github.com/karloscodes/lognorth-sdk-rails). Or point an
OpenTelemetry SDK at it:

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=https://logs.example.com/otel
```

Or post JSON:

```bash
curl -X POST http://localhost:8080/api/v1/events \
  -H "Authorization: Bearer your-app-key" \
  -H "Content-Type: application/json" \
  -d '{
    "message": "POST /api/users → 500",
    "duration_ms": 150,
    "context": {
      "method": "POST",
      "path": "/api/users",
      "status": 500,
      "error": "Database connection timeout"
    }
  }'
```

## MCP

The server answers the Model Context Protocol at `POST /mcp`, so a coding agent
can read production logs while it debugs. It is the only read path for an agent:
there is no REST agent API. Authenticated with the agent key from
Settings > Developer.

```bash
claude mcp add --transport http lognorth http://localhost:8080/mcp \
  --header "Authorization: Bearer lgn-agent-..."
```

Tools: `list_apps`, `list_alerts`, `endpoint_timeline`, `list_issues`,
`search_logs`, `get_event`. All read-only.

The protocol lives in `internal/mcp`; the handler is `internal/http/mcp.go`.

## Development

```bash
make dev      # Run Go API + React dev server
make seed     # Generate sample data with Mailpit config
make build    # Build production binary
make test     # Run the Go tests
```

### Email testing (Mailpit)

The seed command pre-configures Mailpit for local email testing.

```bash
# Start Mailpit
docker compose -f docker-compose.dev.yml up -d
# Seed database (includes Mailpit SMTP config)
make seed
# View emails
open http://localhost:8025
```

**SMTP settings (already configured by seed):**
- Host: `localhost`
- Port: `1025`
- From: `alerts@lognorth.local`
- To: `dev@localhost`

To test: Settings → Send test email → Check Mailpit UI.

### Architecture

See [AGENTS.md](./AGENTS.md) for details.

```
internal/
├── http/       # HTTP handlers
├── events/     # Events & issues
├── apikeys/    # API key management
├── mcp/        # MCP protocol + read-only agent tools
├── settings/   # App configuration
└── jobs/       # Background processing
```

## License

[MIT](./LICENSE)
