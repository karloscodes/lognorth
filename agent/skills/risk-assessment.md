# Skill: Risk assessment (org-level, ships with the agent)

Run AFTER a fix is made, on the diff. Output decides merge strategy.

Classify the change:

**BLOCK to PR + human (never auto-merge), if the diff touches any of:**
- auth / sessions / permissions
- database migrations or schema
- payments / billing
- secrets, env, or key handling
- public API contracts
- dependency additions or upgrades
- deletes of files or data
- a diff larger than ~80 changed lines

**Eligible for auto-merge (only if the owner opted the repo in) when ALL hold:**
- none of the above categories touched
- tests pass (green CI)
- security review passes
- diff is small and localized to the file(s) the issue named

Output: `{ "tier": "auto" | "pr", "reasons": [...] }`

This is a GATE in the orchestrator, not advice the agent may skip. The tier it
returns is what picks PR vs auto-merge.
