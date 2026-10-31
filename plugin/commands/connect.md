---
description: Connect this machine to a LogNorth server so the MCP tools work
argument-hint: [url]
allowed-tools: Bash(north:*), Bash(command -v north)
---

Help the user connect their LogNorth server. The plugin's MCP server is `north mcp`, a small local command that reads the connection from `~/.config/lognorth/remote.json`. `north connect` writes that file.

**Never ask for the agent key in this conversation, and never run a command that contains it.** A key pasted here goes to the AI provider and stays in the transcript. The user runs the connect command in their own terminal, where `north` hides the key as they paste it. If the user pastes a key anyway, do not use it. Tell them to regenerate it in LogNorth, because it is now in the transcript.

## 1. Check the current state

```bash
command -v north && north
```

- **It prints "Reading <host>."**: north is connected. If the `lognorth` tools are missing from your tool list, the user must restart the agent. Stop here.
- **It prints "Not connected yet."** or nothing: continue.

## 2. Give the user one command

Tell the user to open **Settings > Developer** in LogNorth, create an agent key if they have none, and copy the command on that page. It has their URL and key filled in. They run it in a terminal, outside this conversation:

```bash
curl -fsSL https://lognorth.com/cli | sh -s -- <url>
```

Use `$1` as the URL when it is given. Without the key in the command, north asks for it and hides it as they paste it.

The command installs `north` without sudo and checks the key against the server before it saves anything. Then it offers to add LogNorth to Claude Code, Codex, and Gemini CLI.

## 3. After they run it

Run `north` again to confirm the host it reads. Tell the user to restart the agent, because MCP config is read at startup. After that, `/lognorth:investigate` with no argument shows what is wrong in production now. Later key changes need only `north connect`, with no restart.
