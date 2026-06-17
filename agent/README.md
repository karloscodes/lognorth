# Autonomous issue-fixing agent (PoC)

A lognorth feature, not a separate product: lognorth tracks an error → files a
GitHub issue → an agent picks it, fixes it, opens a PR.

**Self-hosted, per-user credentials.** It runs on the user's own box. Each user
brings:
- their own **GitHub auth** (`gh auth login`) — for issues + PRs
- their own **OpenRouter key** (`OPENROUTER_API_KEY`) — BYOK, their account/billing

lognorth never holds shared model credentials. (See note on OpenRouter licensing
below — per-user keys is the model that avoids any reselling question.)

## Architecture (target)

```
Go worker (in lognorth: internal/agent)
  · queue: pick issues labeled agent-fix
  · git: branch in a clone the agent owns (no worktree; one issue at a time)
  · GATES: run tests, then risk-assessment + security review on the diff
  · merge policy: PR by default; auto-merge only in the owner-opted safe zone
      └─ container: opencode (headless `run`)
           · reads the repo's AGENTS.md + skills (project standards)
           · org skills baked into the image (SRE, security, GH, risk)
           · model via OpenRouter (BYOK key)
```

Own the orchestration + gates. Rent the agent loop + skills engine (opencode).
The risk-assessment gate (`skills/risk-assessment.md`) is what classifies a fix
as PR vs auto-merge — it's a gate, not advice the agent may skip.

## This PoC

Proves the riskiest assumption only: **opencode, headless, fixes a real issue
respecting repo standards, verified by tests.** No worker/queue/container/gates
yet — those layer on once this works.

`testdata/fixture-repo/` is a throwaway Go repo with a planted bug + a failing
test + an `AGENTS.md`. The failing test is the success oracle.

### Run it

```bash
export OPENROUTER_API_KEY=sk-or-...        # BYOK
cd agent
./fix.sh                                   # defaults to testdata/fixture-repo
# optional: MODEL=openrouter/<model> ./fix.sh   (see: opencode models openrouter)
```

Expected: opencode reads `AGENTS.md`, fixes `Add` (subtract → add), `go test`
goes green, and the fix is committed on an `agent/fix-*` branch.

## OpenRouter licensing

Per-user own keys = each user is OpenRouter's own customer (their account, key,
billing, ToS). lognorth just makes API calls with the user's credentials — the
standard "plug in your own key" pattern, no reselling. The model to avoid is one
shared key serving everyone. Confirm at openrouter.ai/terms; this shape is the
low-risk one.
