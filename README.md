# LogNorth

[![CI](https://github.com/karloscodes/lognorth/actions/workflows/ci.yml/badge.svg)](https://github.com/karloscodes/lognorth/actions/workflows/ci.yml) [![Release](https://img.shields.io/github/v/release/karloscodes/lognorth)](https://github.com/karloscodes/lognorth/releases/latest) [![Docker pulls](https://img.shields.io/docker/pulls/karloscodes/lognorth)](https://hub.docker.com/r/karloscodes/lognorth) [![License: MIT](https://img.shields.io/github/license/karloscodes/lognorth)](LICENSE)

Logs that alert. Logs you read.

LogNorth is self-hosted error tracking and uptime monitoring in one container and one SQLite file. Your apps send their logs and errors; LogNorth groups them into issues, pings each app every minute, and tells you when something breaks. Your data never leaves your server.

**[Website](https://lognorth.com)** · **[Docs](https://lognorth.com/docs/)**

## Install

On your server, as root:

```bash
curl -fsSL lognorth.com/install | sudo bash -s logs.example.com
```

It installs LogNorth with [Chasen](https://chasenhq.com): HTTPS, live backups of the database, and an update each night. Run it again to update now. The [installation guide](https://lognorth.com/docs/installation/) has the details. To run the image yourself: `karloscodes/lognorth` on Docker Hub.

## What you get

- **Issues** - Errors group into issues. An alert goes out for a new issue, a regression, and a spike, measured against the baseline of your own traffic.
- **Uptime** - A ping to each app every minute, and an alert after three failures in a row.
- **Alerts** - Email, ntfy, Telegram, or a webhook, with quiet hours.
- **Secrets stay out** - A known key format in a log (Stripe, AWS, GitHub, and others) is redacted before LogNorth stores the event, with one alert per app and kind.
- **Agent access** - A coding agent reads production logs over MCP. Every tool is read-only.

## Send events

Use an SDK: [TypeScript](https://github.com/karloscodes/lognorth-sdk-ts), [Go](https://github.com/karloscodes/lognorth-sdk-go), or [Rails](https://github.com/karloscodes/lognorth-sdk-rails). Or point an OpenTelemetry SDK at LogNorth:

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=https://logs.example.com/otel
```

Or post an event as JSON, with the key of the app:

```bash
curl -X POST https://logs.example.com/api/v1/events \
  -H "Authorization: Bearer <app key>" \
  -H "Content-Type: application/json" \
  -d '{"message": "POST /api/users returned 500", "duration_ms": 150, "context": {"method": "POST", "path": "/api/users", "status": 500}}'
```

## MCP

LogNorth serves the Model Context Protocol at `POST /mcp`, with eight read-only tools: `list_apps`, `list_alerts`, `list_endpoints`, `endpoint_timeline`, `uptime_timeline`, `list_issues`, `search_logs`, and `get_event`. It takes the agent key from **Settings > Developer**.

```bash
claude mcp add --transport http lognorth https://logs.example.com/mcp \
  --header "Authorization: Bearer <agent key>"
```

`curl -fsSL https://lognorth.com/cli | sh -s -- <url> <agent key>` adds LogNorth to Claude Code, Codex, Gemini CLI, and Cursor at once. See [the agents guide](https://lognorth.com/agents/).

## Development

```bash
make dev      # the Go API and the React dev server
make test     # the Go tests
make build    # the production binary
make seed     # sample data, with the SMTP settings of Mailpit
```

For email, start Mailpit with `docker compose -f docker-compose.dev.yml up -d`, run `make seed`, and open http://localhost:8025. [AGENTS.md](./AGENTS.md) has the code layout.

## License

MIT. See [LICENSE](LICENSE).
