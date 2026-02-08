# Insights → Lens "Investigate" Link Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Connect Insight cards to Lens with one-click investigation that pre-fills and auto-runs the suggested query.

**Architecture:** Insight cards get an "Investigate" link that navigates to Lens with a query parameter. Lens reads the parameter and auto-submits the question on page load.

**Tech Stack:** Go (Fiber/Cartridge), React, Inertia.js

---

## Task 1: Add Investigate Link to Insight Cards (Frontend)

**Files:**
- Modify: `web/src/components/InsightCard.tsx`

**Step 1: Read current InsightCard component**

```bash
cat web/src/components/InsightCard.tsx
```

Understand current structure and props.

**Step 2: Add investigate link to InsightCard**

The card should show "Investigate →" link when `relatedQuerySuggestion` exists.

```tsx
// Add to InsightCard component, after the description
{insight.relatedQuerySuggestion && (
  <a
    href={`/admin/websites/${websiteId}/lens?q=${encodeURIComponent(insight.relatedQuerySuggestion)}`}
    className="text-sm text-blue-600 hover:text-blue-800 font-medium mt-2 inline-block"
  >
    Investigate →
  </a>
)}
```

**Step 3: Verify visually**

```bash
make dev
```

Open dashboard, check that Insight cards with `relatedQuerySuggestion` show the link.

**Step 4: Commit**

```bash
git add web/src/components/InsightCard.tsx
git commit -m "feat: Add investigate link to insight cards"
```

---

## Task 2: Read Query Parameter in Lens Page (Frontend)

**Files:**
- Modify: `web/src/pages/ProLens.tsx`

**Step 1: Read current ProLens page**

```bash
cat web/src/pages/ProLens.tsx
```

Understand how the page handles state and form submission.

**Step 2: Extract query parameter on mount**

Add URL parameter reading using standard browser API:

```tsx
// Near the top of the component, after existing useState hooks
const [initialQuestion, setInitialQuestion] = useState<string | null>(null);

useEffect(() => {
  const params = new URLSearchParams(window.location.search);
  const q = params.get('q');
  if (q) {
    setInitialQuestion(q);
    // Clean URL without reload
    window.history.replaceState({}, '', window.location.pathname);
  }
}, []);
```

**Step 3: Pass initialQuestion to AskAIChat**

The AskAIChat component already has `externalQuestion` prop. Use it:

```tsx
<AskAIChat
  // ... existing props
  externalQuestion={initialQuestion}
  onExternalQuestionConsumed={() => setInitialQuestion(null)}
/>
```

**Step 4: Verify manually**

```bash
make dev
```

1. Go to `/admin/websites/1/lens?q=test%20question`
2. Verify the textarea shows "test question"

**Step 5: Commit**

```bash
git add web/src/pages/ProLens.tsx
git commit -m "feat: Lens reads question from URL parameter"
```

---

## Task 3: Auto-Submit Question on Load (Frontend)

**Files:**
- Modify: `web/src/pages/ProLens.tsx`

**Step 1: Add auto-submit behavior**

When `initialQuestion` is set, automatically submit the form after a short delay:

```tsx
// Add new state for auto-submit
const [shouldAutoSubmit, setShouldAutoSubmit] = useState(false);

// Modify the URL parameter effect
useEffect(() => {
  const params = new URLSearchParams(window.location.search);
  const q = params.get('q');
  if (q) {
    setInitialQuestion(q);
    setShouldAutoSubmit(true);
    window.history.replaceState({}, '', window.location.pathname);
  }
}, []);

// Add effect to trigger auto-submit
useEffect(() => {
  if (shouldAutoSubmit && initialQuestion && !isLoading) {
    setShouldAutoSubmit(false);
    handleAskAI(initialQuestion, currentWebsiteId);
  }
}, [shouldAutoSubmit, initialQuestion, isLoading, currentWebsiteId]);
```

**Step 2: Verify full flow**

```bash
make dev
```

1. Go to Dashboard with an Insight card
2. Click "Investigate →"
3. Verify: Lens opens, question fills, query auto-runs, results show

**Step 3: Commit**

```bash
git add web/src/pages/ProLens.tsx
git commit -m "feat: Auto-submit question when coming from Insights"
```

---

## Task 4: Ensure websiteId is Available for Link (Frontend)

**Files:**
- Modify: `web/src/components/InsightCard.tsx` (if needed)
- Modify: Parent component that renders InsightCard

**Step 1: Check if InsightCard receives websiteId**

```bash
grep -n "websiteId" web/src/components/InsightCard.tsx
grep -n "InsightCard" web/src/pages/ProDashboard.tsx
```

**Step 2: Add websiteId prop if missing**

If InsightCard doesn't have websiteId, add it to the props interface:

```tsx
interface InsightCardProps {
  insight: Insight;
  websiteId: number; // Add if missing
}
```

And pass it from the parent component.

**Step 3: Verify link works**

```bash
make dev
```

Click "Investigate →" and confirm URL contains correct website ID.

**Step 4: Commit (if changes made)**

```bash
git add web/src/components/InsightCard.tsx web/src/pages/ProDashboard.tsx
git commit -m "feat: Pass websiteId to InsightCard for investigate link"
```

---

## Task 5: Run E2E Tests

**Files:**
- None (verification only)

**Step 1: Run existing E2E tests**

```bash
make test-e2e
```

Ensure nothing is broken.

**Step 2: Fix any failures**

If tests fail, investigate and fix.

**Step 3: Commit fixes if any**

```bash
git add -A
git commit -m "fix: E2E test fixes for investigate link feature"
```

---

## Task 6: Final Commit and Summary

**Step 1: Verify all changes**

```bash
git log --oneline -5
git status
```

**Step 2: Create final summary commit if needed**

If there are uncommitted changes:

```bash
git add -A
git commit -m "feat: Complete Insights → Lens investigate link feature"
```

---

## Testing Checklist

- [ ] Insight card shows "Investigate →" when `relatedQuerySuggestion` exists
- [ ] Insight card without `relatedQuerySuggestion` shows no link
- [ ] Clicking link navigates to Lens with `?q=` parameter
- [ ] Lens reads parameter and fills textarea
- [ ] Lens auto-submits the question
- [ ] URL is cleaned after reading parameter (no `?q=` visible)
- [ ] Results display correctly
- [ ] E2E tests pass
