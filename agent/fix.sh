#!/usr/bin/env bash
set -euo pipefail

# PoC: drive opencode headless to fix an issue, verify via the repo's own tests.
#
# Self-hosted model: this runs on the USER's machine/box. Each user brings:
#   - their own GitHub auth   (gh auth login)        — for issues + PRs
#   - their own OpenRouter key (OPENROUTER_API_KEY)  — BYOK, their account/billing
# The platform never holds shared model credentials.

REPO="${1:-testdata/fixture-repo}"
MODEL="${MODEL:-openrouter/anthropic/claude-sonnet-4}"   # change: opencode models openrouter
TEST_CMD="${TEST_CMD:-go test ./...}"
BRANCH="agent/fix-$(date +%s)"

if [ -z "${OPENROUTER_API_KEY:-}" ]; then
  echo "Set your OpenRouter key first (BYOK):  export OPENROUTER_API_KEY=sk-or-..."
  exit 1
fi

cd "$REPO"
ISSUE="$(cat ISSUE.md)"

git checkout -b "$BRANCH"

echo "=== opencode (headless) is fixing the issue ==="
# opencode auto-reads this repo's AGENTS.md (project standards). Org skills would
# layer in via the agent image / global config in the real build.
opencode run -m "$MODEL" "You are fixing a bug in this repository. Read AGENTS.md and obey it.

Issue:
$ISSUE

Make the fix, then run '$TEST_CMD' and make sure it passes. Touch only what's needed."

echo "=== verifying: $TEST_CMD ==="
if $TEST_CMD; then
  git add -A
  git commit -q -m "fix: $(head -1 ISSUE.md | sed 's/^# //')"
  echo "PASS — fix committed on branch $BRANCH"
  echo "(next step: gh pr create  — once this points at a real GitHub repo)"
else
  echo "FAIL — tests still red. Branch $BRANCH left for inspection."
  exit 1
fi
