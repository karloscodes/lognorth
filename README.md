# LogNorth

Know when things break. Self-hosted error tracking.

## Quick Start

```bash
make dev      # Run Go API + React dev server
make seed     # Generate sample data with Mailpit config
make build    # Build production binary
```

## Development

### Email Testing (Mailpit)

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

### Running Tests

```bash
make test
```

## Architecture

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

## Event Ingestion

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

Tools: `list_apps`, `search_logs`, `get_event`, `list_issues`. All read-only.
The protocol lives in `internal/mcp`; the handler is `internal/http/mcp.go`.
