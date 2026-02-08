# Lens Example Questions - Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Show clickable example question chips below the Lens input, always visible regardless of saved queries.

**Architecture:** Modify AskAIChat component to always show compact suggestion chips. Remove the duplicate help overlay from ProLens.tsx. Consolidate to a single source of example questions.

**Tech Stack:** React, TypeScript, Tailwind CSS

---

## Task 1: Update AskAIChat to Always Show Suggestions

**Files:**
- Modify: `/Users/karloscodes/Code/im/fusionaly-pro/web/src/components/AskAIChat.tsx`

**Step 1: Change condition to always show example questions**

Current condition (line 193):
```tsx
{!hasSavedQueries && !aiResult && (
```

Change to:
```tsx
{!aiResult && (
```

**Step 2: Make chips more compact**

Replace lines 194-213 with compact horizontal chip layout:

```tsx
{!aiResult && (
  <div className="mt-4 flex flex-wrap items-center gap-2">
    <span className="text-xs text-gray-500">Try:</span>
    {EXAMPLE_QUESTIONS.slice(0, 4).map((example, index) => (
      <button
        key={index}
        onClick={() => handleExampleClick(example)}
        disabled={isLoading}
        className="text-xs px-3 py-1.5 bg-gray-100 hover:bg-gray-200 text-gray-700 hover:text-gray-900 rounded-full transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
      >
        {example}
      </button>
    ))}
  </div>
)}
```

**Step 3: Run type check**

Run: `cd web && npx tsc --noEmit`
Expected: No errors

**Step 4: Commit**

```bash
git add web/src/components/AskAIChat.tsx
git commit -m "feat: Always show example question chips in Lens"
```

---

## Task 2: Remove Duplicate Help Overlay from ProLens

**Files:**
- Modify: `/Users/karloscodes/Code/im/fusionaly-pro/web/src/pages/ProLens.tsx`

**Step 1: Remove HELP_EXAMPLE_QUESTIONS constant**

Delete lines 96-105:
```tsx
// Example questions for the help overlay
const HELP_EXAMPLE_QUESTIONS = [
  "What are the top 10 pages by views?",
  // ...
];
```

**Step 2: Remove showHelpOverlay state**

Delete line ~163:
```tsx
const [showHelpOverlay, setShowHelpOverlay] = useState(false);
```

**Step 3: Remove handleHelpExampleClick function**

Delete lines 208-214:
```tsx
const handleHelpExampleClick = (question: string) => {
  setShowHelpOverlay(false);
  setPendingQuestion(question);
  handleAISubmit(question, websiteId);
};
```

**Step 4: Remove help icon button from header**

Delete the TooltipProvider wrapping the HelpCircle button (lines ~527-541).

Keep only:
```tsx
<div className="flex items-center gap-3">
  <div className="flex items-center gap-2">
    <InfinityIcon className="h-6 w-6" />
    <h1 className="text-2xl font-bold text-gray-900">Lens</h1>
  </div>
</div>
```

**Step 5: Remove the help overlay JSX**

Delete the entire help overlay section (lines ~546-578).

**Step 6: Remove unused import**

Remove `HelpCircle` from lucide-react imports if no longer used.

**Step 7: Run type check**

Run: `cd web && npx tsc --noEmit`
Expected: No errors

**Step 8: Commit**

```bash
git add web/src/pages/ProLens.tsx
git commit -m "refactor: Remove duplicate help overlay from Lens"
```

---

## Task 3: Remove hasSavedQueries Prop (No Longer Needed)

**Files:**
- Modify: `/Users/karloscodes/Code/im/fusionaly-pro/web/src/components/AskAIChat.tsx`
- Modify: `/Users/karloscodes/Code/im/fusionaly-pro/web/src/pages/ProLens.tsx`

**Step 1: Remove hasSavedQueries from AskAIChat interface**

In `AskAIChat.tsx`, remove from interface:
```tsx
hasSavedQueries: boolean;
```

**Step 2: Remove from destructured props**

Remove `hasSavedQueries` from:
```tsx
}) => {
```

**Step 3: Remove from ProLens.tsx usage**

In `ProLens.tsx`, remove the prop from AskAIChat usage:
```tsx
hasSavedQueries={savedQueries.length > 0}
```

**Step 4: Run type check**

Run: `cd web && npx tsc --noEmit`
Expected: No errors

**Step 5: Commit**

```bash
git add web/src/components/AskAIChat.tsx web/src/pages/ProLens.tsx
git commit -m "refactor: Remove hasSavedQueries prop from AskAIChat"
```

---

## Task 4: Verify with E2E Tests

**Step 1: Run E2E tests**

Run: `make test-e2e`
Expected: All tests pass

**Step 2: Manual verification**

1. Start dev server: `make dev`
2. Navigate to Lens page
3. Verify: Example chips visible below input
4. Click a chip: Query executes
5. Save a query
6. Verify: Example chips still visible after having saved queries
