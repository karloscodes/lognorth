# Feature: Rename Apps

## Goal
Let users fix app names without revoking their API key and re-integrating.

## Behavior
- Apps page (`/apps` or settings) shows an inline "Edit" affordance next to each app name
- Clicking switches to a text input + Save / Cancel
- POST `/apps/:id/rename` with `{ name: "..." }` updates `api_keys.name`
- Validation: 1–60 chars, trimmed, must be unique among that user's active keys
- On success: flash "Renamed", refresh app list (and the dropdown in the layout)
- On conflict: flash error, keep edit input open

## Acceptance
- [ ] Edit button visible per-app on the apps/settings list
- [ ] Empty or duplicate names show inline error, no DB write
- [ ] Existing API key, ingestion, and stored events are unaffected (only `name` changes)
- [ ] App selector in nav reflects new name immediately after save
- [ ] Demo mode still blocks the POST (no-op for read-only demo)

## Notes
- `apikeys.Update` likely already exists — verify before adding new method.
- Frontend: tiny inline form, no modal.
