# Feature: Request Path Enrichment (Controller / Action / Handler)

## Goal
The Logs detail page currently shows just the path (e.g. `GET /conversations`). Add the *route name* — Rails controller#action, Go handler func name, Node route handler — so users can jump from a slow endpoint to the code that served it.

## Behavior

### SDK side
Each SDK enriches the request event's `context` with framework-specific identifiers:

**Rails** (middleware sees `env["action_controller.instance"]` after dispatch):
```json
{
  "controller": "ConversationsController",
  "action": "index",
  "route": "conversations#index"
}
```

**Go** (chi/echo/fiber expose route patterns):
```json
{
  "route": "/conversations/:id",
  "handler": "main.showConversation"
}
```

**Node/Express** (req.route + handler.name):
```json
{
  "route": "/conversations/:id",
  "handler": "showConversation"
}
```

Best-effort — if the framework doesn't expose it (e.g. raw net/http), the field is omitted, no error.

### Server / UI side
- `EventContext` gains optional `Controller`, `Action`, `Route`, `Handler` fields
- Log detail page renders a small block under the path:

  ```
  GET 200  /conversations          20ms
  ConversationsController#index
  ```

- If only `route` is present, show that. If only `handler`, show that. Hide block if all empty.
- No new DB column — the data lives in `events.context` JSON, displayed by reading from it.

## Acceptance
- [ ] Rails SDK populates `controller` and `action` from the dispatched controller instance
- [ ] Go SDK middleware example (chi) populates `route` and `handler`
- [ ] TS/Express middleware populates `route` and `handler`
- [ ] Log detail UI shows the enrichment when present, hides when absent
- [ ] No regression for events from old SDK versions (fields just absent)
- [ ] Existing event grouping/issue hashes unchanged

## Notes
- This addresses the "feels empty" detail page from the screenshot.
- Optional follow-up: add Controller#Action as a filter on the Logs page. Out of scope for this spec.
