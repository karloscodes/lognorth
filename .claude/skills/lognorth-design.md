---
name: lognorth-design
description: Use when building UI components, pages, or styling for LogNorth - covers the design system, color palette, typography, and component patterns
---

# LogNorth Design System

The app is a dark terminal UI that matches lognorth.com. CLAUDE.md, section
"Terminal Theme", is the source of truth for tokens, components, and keys.
This file is the quick version.

## Terminal Theme

Dark only, and it reads like a terminal session. Same palette and font as
lognorth.com. Tokens live in `web/src/index.css`; use them through Tailwind,
never as hex values.

| Token (Tailwind) | Use |
|---|---|
| `bg-background` | page (#0c0a09) |
| `bg-raise` | selected row, hover, status bar (#1c1917) |
| `text-foreground` | headings, primary values |
| `text-text` | body copy |
| `text-subtle` | secondary text |
| `text-dim` | tertiary text; the floor for readable text (~5.3:1) |
| `border-border` / `border-line` | strong line (panes) / hairline (rows) |
| `text-error` / `bg-error` | errors only (#e11d48) |
| `text-accent` / `bg-accent` | lime (#a3e635): wordmark brackets, active tab brackets, live dots, key hints in navigate mode |

- **No gray, no hex:** never `gray-*` or `text-[#...]`. Use the tokens above.
- **Red means error.** Never decorative. An endpoint turns red only when it
  alerts against its own history (`events.ListAlerts`), not by a fixed rate.
- **No rounded corners.** `--radius` is 0. Radios and checkboxes are squares.
- **Font:** IBM Plex Mono only, bundled in `web/src/fonts/` (OFL) and loaded by
  `@font-face` in `index.css`. Vite hashes the files into `/assets`, so a
  self-hosted install makes no third-party font requests. Max weight is 700.
  Ligatures are off so `->` and `!=` stay literal.

## Components

- **Pane:** `.pane` with a `.pane-title` (and optional `.pane-right`) set into
  the top border: `<div className="pane"><span className="pane-title">stack trace</span>...`
- **Section label:** `.rule-label` renders `active  last 7 days ─────`.
- **Key chip:** `.key` with a `<kbd>`: `<button className="key"><kbd>m</kbd> mute</button>`.
- **Toggles:** the active option wears lime brackets: `[errors]`, `[7d]`.
- **Page header:** `PageHeader` in `lib/shared.tsx` (26px bold title, subtle subtitle).
- **Tabs:** `1 [issues] 2 logs 3 endpoints`; the active tab gets lime brackets.
- **Stack traces:** `components/stack-trace.tsx` shows app frames and folds
  library frames (`lib/stack.ts` decides which is which).

## Keyboard (herdr style)

- **Prefix:** `ctrl+b` opens navigate mode (the layout owns it). Tab numbers
  light up and hint badges appear on every `data-key` element.
  Then `1-3` tabs, `h/l` move, `enter` go, `0` home, `a` apps, `s` settings,
  `w` app picker, `?` keys, `esc` cancel.
- **Normal mode:** keys go to what is on screen. Pages add their own with
  `pageKey(e)` from `lib/keys.ts`, which stays quiet while typing or while
  navigate mode is open. Issues: `j/k enter m e E`. Issue and log pages: `esc`.
  Logs: `/`.
- **Status bar** (`layout.tsx`, desktop only): mode, screen, and the keys that
  work right now. Add a screen's keys to `SCREEN_KEYS`.
- **Command line:** `:` opens a vim-style line in the status bar
  (`components/command-line.tsx`). Commands that work anywhere live in
  `COMMANDS` in `lib/commands.ts`; a page adds its own while it is on screen
  with `usePageCommand('mute', fn)`. A command returns nothing or a status
  line, and a line that starts with `E` shows red. Tab completes, ↑ ↓ recall.

## Voice

Short and plain. "X times", not "occurrences". "Quiet so far", "Nothing here",
"Enjoy your day." for empty states. Lowercase labels in panes, tabs, and the
status bar; sentence case for headings and messages.
