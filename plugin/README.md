# LogNorth plugin

Your agent should not have to ask you what production is doing. This plugin connects it straight to your [LogNorth](https://lognorth.com) server: it reads the failing requests, follows the trace, and tells you what broke, in the same pane as the code.

It bundles two things. The **MCP server** gives your agent eight read-only tools over your logs and alerts. The **skills** teach it how to go from an alert to the line of code and the commit that caused it.

Everything stays on your box. Your agent asks your instance, and only the answer reaches your AI provider.

It installs as a plugin in Claude Code, Codex, GitHub Copilot CLI, VS Code, and Gemini CLI, from one terminal command.

Needs LogNorth **v0.16.0 or later**. `list_alerts`, `endpoint_timeline`, and `/lognorth:investigate` need the release after v0.16.2. `list_endpoints` and `uptime_timeline` need v0.20.0. To update, run the install line on the server again: `curl -fsSL lognorth.com/install | bash` (`lognorth update` on a server of the older installer).

## What you get

| Tool | What it does |
|------|--------------|
| `list_apps` | Your apps and their ids |
| `list_alerts` | What is alerting now: error rate spikes, traffic drops, apps down, each against its normal level |
| `list_endpoints` | Every endpoint with its requests, errors, and error rate, most broken first |
| `endpoint_timeline` | Requests and errors for one endpoint over time, to see when a problem started |
| `uptime_timeline` | The app's uptime ping over 24 hours, and why pings failed |
| `list_issues` | Grouped errors with occurrence counts and trend |
| `search_logs` | Find requests by text, issue, errors only, and time window |
| `get_event` | One event with its full context and every event in its trace |

| Skill | What it does |
|-------|--------------|
| `lognorth` | Investigate production: from an alert to the failing request, the code, and the commit |
| `lognorth-integrate` | Add the LogNorth SDK to a Go, Node, or Rails project |

| Command | What it does |
|---------|--------------|
| `/lognorth:connect` | Check the connection, and say what to run when it is missing |
| `/lognorth:investigate` | Investigate an alert or issue. Every LogNorth alert email ends with this command, ready to paste |

There is no tool that writes, mutes, or deletes. The agent can look, never touch.

## Install

One command in your terminal. Copy it from **Settings > Developer** in LogNorth, where it has your URL and agent key filled in:

```bash
curl -fsSL https://lognorth.com/cli | sh -s -- https://logs.yoursite.com lgn-agent-...
```

It installs `north`, a small open-source binary. Then it checks the key against your server, saves the connection to `~/.config/lognorth/remote.json` (readable by you only), and offers to add LogNorth to every agent it finds:

```
✓ Connected to logs.yoursite.com: shop-prod, shop-staging
  Saved to ~/.config/lognorth/remote.json, readable by you only

Add LogNorth to Claude Code, Codex, Gemini CLI and Cursor? [Y/n]
  ✓ Claude Code  plugin lognorth
  ✓ Codex        plugin lognorth
  ✓ Gemini CLI   extension lognorth
  ✓ Cursor       ~/.cursor/mcp.json

Restart your agent, then ask it: what is broken in production?
```

The agent key is read-only and starts with `lgn-agent-`. It is not the app key your SDKs use to send events.

**How it connects.** Every agent starts `north mcp`, which relays its calls to your server over HTTPS. The URL and the key live in that one file, never in an agent config or a dotfiles repo. A new key is one `north connect` away, and running agents pick it up on their next call.

Install an agent later? Run `north agents`.

### From the plugin marketplace

You can also install the plugin first: `/plugin marketplace add karloscodes/lognorth-releases`, then `/plugin install lognorth`. It still connects through the terminal command above, so the key never goes through the chat. `/lognorth:connect` checks the connection and tells you what to run.

### Other agents

After `north connect`, any MCP client takes the same local command:

```json
{
  "mcpServers": {
    "lognorth": { "command": "north", "args": ["mcp"] }
  }
}
```

- **Cursor**: `north agents` writes the JSON above into `~/.cursor/mcp.json`. The Cursor Marketplace plugin connects by URL instead: set `LOGNORTH_URL` and `LOGNORTH_AGENT_KEY` under **Plugins > Configure** in the Cursor dashboard. That also works for Cursor cloud agents, where `north` cannot run.
- **GitHub Copilot CLI**: `copilot plugin marketplace add karloscodes/lognorth-releases`, then `copilot plugin install lognorth@karloscodes`.
- **VS Code with Copilot**: add `"chat.plugins.marketplaces": ["karloscodes/lognorth-releases"]` to your settings, then install **lognorth** from the Extensions view (search `@agentPlugins`).
- **Windsurf**: the JSON above, in `~/.codeium/windsurf/mcp_config.json`.
- **Zed**: in `settings.json`, `"context_servers": { "lognorth": { "source": "custom", "command": { "path": "north", "args": ["mcp"] } } }`.

An app started from the Dock may not see your shell's `PATH`. If it cannot find `north`, use the full path from `command -v north`.

### Without north

The server speaks MCP over HTTPS at `https://logs.yoursite.com/mcp`, with the agent key as a Bearer token. Any client that takes a URL and headers can connect directly:

```bash
claude mcp add --transport http lognorth https://logs.yoursite.com/mcp \
  --header "Authorization: Bearer lgn-agent-..."
```

That puts the key in the client's config file. Keep that file out of git.

For an agent without plugins, copy `skills/lognorth` and `skills/lognorth-integrate` into its skills folder.

## Then ask

An alert email ends with a line like this. Paste it into your agent:

```
/lognorth:investigate /api/checkout

Agent: [list_alerts, endpoint_timeline, search_logs, get_event]

/api/checkout fails 8% of the time since 14:05, normally 1%. All 212
failures are "Stripe timeout after 2s" at app/payments/charge.rb:41.

[git log around 14:05]

a3f9c1e, deployed 14:02, cut the Stripe timeout from 30s to 2s.
Stripe's p99 today is 3.1s. Restore the 30s timeout; the diff is below.
```

Or just ask: "is anything broken in production?"

## If it does not connect

Run `north` in a terminal. It says which server it reads, or that it is not connected yet.

- **"the server rejected the agent key"**: a wrong key, or an app key. Run `north connect` with the agent key from Settings > Developer.
- **404 on `/mcp`**: the server predates v0.16.0. Update it: run the install line on it again, or `lognorth update` on a server of the older installer.
- **Nothing in the tool list**: most clients read MCP config at startup. Restart the agent.
- **"north: command not found" in the agent's MCP log**: the agent cannot see `north` on its `PATH`. Add the folder the installer named to your `PATH`, or put the full path in the config.

## From your terminal

`north` also reads your server without an agent:

```bash
north tail --errors     # the log, live
north top               # endpoints, alerts, and uptime, like htop
north call list_alerts  # any tool, as JSON
north update            # the latest north
```

Source: [karloscodes/lognorth-cli](https://github.com/karloscodes/lognorth-cli). See [Terminal](https://lognorth.com/docs/features/terminal/).

## Docs

- [MCP setup and every client's config](https://lognorth.com/docs/integrations/mcp/)
- [All docs](https://lognorth.com/docs/)

## Releases

This repo also carries the LogNorth binary releases the installer downloads. See [Releases](https://github.com/karloscodes/lognorth-releases/releases).
