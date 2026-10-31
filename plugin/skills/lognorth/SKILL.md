---
name: lognorth
description: Use when debugging production errors, investigating an alert, checking logs, or the user mentions "lognorth". Reads a self-hosted LogNorth instance through its MCP tools, then finds the cause in the code.
---

# LogNorth

The user's production log is available through the `lognorth` MCP tools. You also have their code and its git history. A hosted log tool has only the first. Use both: the log says what broke and when, the repo says why.

All tools are read-only. Nothing here writes, mutes, or deletes.

## Investigate

Go in this order. Stop as soon as you know the cause.

1. **What is wrong now.** `list_alerts` for rate alerts (spike, drift, silence) and apps that are down. `list_issues` for grouped errors. `list_endpoints` for which endpoints fail right now, most broken first. `uptime_timeline` when an app may be down: when its pings failed, and why; its `outages` name the issues whose errors surged in each outage, the likely cause. When the user names a path or an issue, start from it. An issue in `list_issues` with `after` is a symptom: in most of its failed requests, the issue in `after` failed first. Investigate that one.
2. **When it started.** `endpoint_timeline` for the path. Find the first step where errors rose or traffic fell, and compare it with the normal level in the response.
3. **What failed.** `search_logs` with `errors_only: true`, `issue: <hash>` when you have one, and `since` / `until` around the start time.
4. **The whole request.** `get_event` on one failure. Read the trace: what succeeded just before the failure rules out half the causes. When the event has a user, `stepsBefore` lists that user's requests in the 30 minutes before it: use them as the steps to reproduce.
5. **The code.** Open `error_file:error_line` from the event. When its context has `release`, that version ran: look for the change in it first. Otherwise run `git log --since=<start minus 1 hour> --until=<start>` and look for the change that shipped just before the problem began.
6. **The fix.** Name the cause, cite the event id and the commit, and propose the change. Do not apply it unless the user asks.

Call `list_apps` when a tool needs an `app_id`. One app: use it silently. Several, and the user did not say which: ask.

## Rules

- **Lead with the answer**, not the JSON. "Checkout fails 8% of the time since 14:05, normally 1%. All Stripe timeouts, starting with a3f9c1e, which cut the timeout to 2s" beats a table.
- **Trust the alert's baseline.** An alert compares against the same time of day on earlier days, so it is already more than "errors went up". Quote its normal level.
- **Check the volume before calling it urgent.** One error in a thousand requests is normal. An issue with a high 7-day count and a falling trend is fading, so say that.
- **Say when you are guessing.** No trace, no `error_file`, or no commit near the start time means the cause is a hypothesis. Label it.
- **Never claim you fixed, muted, or resolved anything in LogNorth.** You can only read.

## Without the MCP tools

If the `lognorth` tools are not in your tool list, the same tools run from the shell with `north`:

```bash
north call list_alerts
north call search_logs '{"errors_only": true, "since": "2h"}'
```

- It prints the tool's JSON answer. The tools and their arguments are the ones the steps above name.
- If `north` is missing or says it is not connected, tell the user to copy the command from **Settings > Developer** in LogNorth and run it in their terminal. Never ask for the agent key in the chat.

## When a tool is missing

`after` in `list_issues`, `stepsBefore` in `get_event`, and `outages` in `uptime_timeline` need v0.27.3. `list_alerts`, `endpoint_timeline`, and the `issue` and `until` arguments of `search_logs` need a newer LogNorth, and `list_endpoints` and `uptime_timeline` need v0.20.0. If they are not in the tool list, tell the user to update LogNorth (run the install line `curl -fsSL lognorth.com/install | bash` on the server again, or `lognorth update` on a server of the older installer), then continue with the tools you have. If no `lognorth` tools appear at all, use `north call` as above, and tell the user that `north agents` in a terminal adds the tools.

## More detail

Read [reference.md](reference.md) when you need it: what each field means, how alerts decide, and how traces fit together.
