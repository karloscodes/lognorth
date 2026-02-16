---
name: lognorth-design
description: Use when building UI components, pages, or styling for LogNorth - covers the design system, color palette, typography, and component patterns
---

# LogNorth Design System

## Color Palette

### Primary Palette

| Use | Class |
|-----|-------|
| Primary text | `text-black` |
| Secondary text | `text-black/50` |
| Background | `bg-white` |
| Subtle background | `bg-black/[0.02]` |
| Border | `border-black/10` |
| Border hover | `border-black/20` |
| Divider | `border-black/10` or `h-px bg-black/20` |

### Accent Color

| Use | Value |
|-----|-------|
| Success/healthy | `#00D678` (green) |
| Success dot | `bg-[#00D678]` |
| Success text | `text-[#00D678]` |

### Semantic Colors

| Use | Class |
|-----|-------|
| Error text | `text-red-600` |
| Error background | `bg-red-50` |
| Error border | `border-red-200` |
| Error dot | `bg-red-500` |

### NEVER Use

- Gray classes (`text-gray-*`, `bg-gray-*`)
- Intermediate opacities (`black/60`, `black/70`, `black/80`)
- `rounded-xl` or `rounded-lg` on containers

## Typography

| Element | Classes |
|---------|---------|
| Page title | `text-2xl font-bold text-black` |
| Section header | `text-sm font-semibold text-black/50 uppercase tracking-wider` |
| Card title | `text-lg font-semibold text-black` |
| Labels | `text-sm font-semibold text-black` |
| Body | `text-black` |
| Secondary | `text-black/50` |
| Timestamp | `text-xs text-black/50` |

Rules:
- Two text levels only: `text-black` or `text-black/50`
- Use `font-semibold` liberally
- Monospace for: paths, status codes, counts, durations

## Components

### Buttons

```tsx
// Primary
<button className="px-4 py-2 bg-black text-white font-semibold hover:bg-black/80">

// Secondary
<button className="px-4 py-2 border border-black/20 font-semibold text-black/50 hover:border-black hover:text-black">

// Ghost
<button className="px-4 py-2 text-black/50 font-semibold hover:text-black hover:bg-black/[0.02]">
```

### Inputs

```tsx
<label className="block text-sm font-semibold text-black mb-1.5">Label</label>
<input className="w-full px-3 py-2 border border-black/20 text-sm focus:outline-none focus:ring-1 focus:ring-black placeholder:text-black/50" />
```

### Cards

```tsx
// Standard (sharp corners)
<div className="bg-white border border-black/10 p-5">

// List with dividers
<div className="bg-white border border-black/10 divide-y divide-black/10 overflow-hidden">

// Error state
<div className="border border-red-200 bg-red-50 p-5">

// Empty state
<div className="border border-dashed border-black/20 py-16 text-center">
  <p className="font-semibold text-black mb-1">No items</p>
  <p className="text-sm text-black/50">Description.</p>
</div>
```

### Status Indicators

```tsx
// Success dot
<span className="w-2 h-2 rounded-full bg-[#00D678]" />

// Error dot
<span className="w-2 h-2 rounded-full bg-red-500" />

// Live/pulsing
<span className="w-1.5 h-1.5 rounded-full bg-red-500 animate-pulse" />
```

### Navigation Tabs (pills get rounding)

```tsx
<nav className="flex items-center gap-1 bg-black/[0.02] rounded-lg p-1">
  <Link className={isActive
    ? 'px-4 py-2 text-sm font-semibold rounded-md bg-white text-black shadow-sm'
    : 'px-4 py-2 text-sm font-semibold rounded-md text-black/50 hover:text-black'
  }>
```

### List Rows

```tsx
// Clickable row
<a className="flex items-center justify-between p-5 hover:bg-black/[0.02]">

// Error indicator (left border)
<a className={`p-5 ${isError ? 'border-l-2 border-l-red-500' : ''}`}>

// Unseen/new
<a className={`p-5 ${isUnseen ? 'border-l-2 border-l-red-500 bg-red-50' : ''}`}>
```

### Messages

```tsx
// Success
<div className="p-3 text-sm font-medium bg-[#00D678]/10 text-black">

// Error
<div className="p-3 text-sm font-medium bg-red-50 text-red-600">
```

## HTTP/Status Colors

```tsx
// Methods - no color decoration, just monospace
<span className="font-mono text-xs font-semibold text-black/50">{method}</span>

// Status codes - only these carry semantic weight
500+: 'text-red-600 font-semibold'
400+: 'text-black'
200+: 'text-black/50'
```

## Layout

```tsx
// Page header
<div className="flex items-center justify-between mb-8">
  <div>
    <h1 className="text-2xl font-bold text-black">Title</h1>
    <p className="text-sm text-black/50 mt-1">Description</p>
  </div>
</div>

// Form container
<div className="max-w-md">
  <div className="space-y-6">...</div>
  <div className="flex gap-3 mt-6">...</div>
</div>
```

## Spacing

| Use | Class |
|-----|-------|
| Section gap | `mb-8` |
| Element gap | `space-y-4` |
| Card padding | `p-5` |
| Button padding | `px-4 py-2` |

## Rules

1. **Two opacity levels only:** `text-black` (important) or `text-black/50` (not)
2. **Sharp containers:** No `rounded-xl` on cards/forms. Only pills/badges get rounding.
3. **One border width:** Use `border`, never `border-2`
4. **Status codes carry meaning:** HTTP methods are monospace, no colors
5. **No gray classes:** Use `black/*` opacity instead
