import { useEffect, useRef, useState } from 'react'

// Deploys on a time chart: a dashed line with a ▼ head where each release
// first started, so it reads as a deploy and not as a grid line. Deploys too
// close to tell apart share one mark ("v1.4.2 +2"), and a mark has a label
// when the label has room. Place it inside the chart's bars, in a relative
// box; it ignores the mouse so the bars keep their hover, and the hover text
// names the deploys of a bar (releasesIn).

export interface Release {
  version: string
  first_seen: string
}

// Deploys closer than this, in pixels, share one mark.
const MERGE_PX = 12
// A label is 10px IBM Plex Mono, 6px a character. It sits 8px from its line
// and keeps 16px clear of the next, so it never reads as the next mark's.
const labelPx = (label: string) => label.length * 6 + 24

interface Mark {
  at: number
  versions: string[]
}

// A git SHA as its first 7 characters; any other release name as it is.
export function shortRelease(release: string): string {
  return /^[0-9a-f]{12,}$/i.test(release) ? release.slice(0, 7) : release
}

// The deploys that started in [start, end), for a bar's hover text.
export function releasesIn(releases: Release[] | null | undefined, start: number, end: number): string[] {
  return (releases ?? [])
    .filter((r) => {
      const at = new Date(r.first_seen).getTime()
      return at >= start && at < end
    })
    .map((r) => shortRelease(r.version))
}

function marksOf(releases: Release[], from: number, span: number, merge: number): Mark[] {
  const shown = releases
    .map((r) => ({ version: r.version, at: (new Date(r.first_seen).getTime() - from) / span }))
    .filter((r) => r.at >= 0 && r.at <= 1)
    .sort((a, b) => a.at - b.at)
  const marks: Mark[] = []
  for (const r of shown) {
    const last = marks[marks.length - 1]
    if (last && r.at - last.at < merge) last.versions.push(r.version)
    else marks.push({ at: r.at, versions: [r.version] })
  }
  return marks
}

// The newest version of a mark, and how many others share it: "v1.4.2 +2".
function labelOf(mark: Mark): string {
  const newest = shortRelease(mark.versions[mark.versions.length - 1])
  return mark.versions.length > 1 ? `${newest} +${mark.versions.length - 1}` : newest
}

export function ReleaseMarks({ releases, from, to }: { releases?: Release[] | null; from: number; to: number }) {
  const box = useRef<HTMLDivElement>(null)
  const [width, setWidth] = useState(0)
  useEffect(() => {
    if (!box.current) return
    const observer = new ResizeObserver(([entry]) => setWidth(entry.contentRect.width))
    observer.observe(box.current)
    return () => observer.disconnect()
  }, [])

  // The box renders even without deploys, so it has a width when they come.
  const span = to - from
  const marks = releases?.length && span > 0 && width > 0 ? marksOf(releases, from, span, MERGE_PX / width) : []

  return (
    <div ref={box} className="pointer-events-none absolute inset-0" aria-hidden="true">
      {placeLabels(marks, width).map(({ mark, label, flip }) => (
        <div key={mark.at} className="absolute bottom-0 top-0 border-l border-dashed border-text" style={{ left: `${mark.at * 100}%` }}>
          <span className="absolute -left-[4.5px] -top-3 text-[8px] leading-none text-foreground">▼</span>
          {label && (
            <span className={`absolute -top-3 whitespace-nowrap text-[10px] leading-none text-text ${flip ? 'right-2' : 'left-2'}`}>{label}</span>
          )}
        </div>
      ))}
    </div>
  )
}

// A label goes right of its line, or left of it near the right edge, when
// it fits before the next line and after the label before it.
function placeLabels(marks: Mark[], width: number): { mark: Mark; label?: string; flip: boolean }[] {
  let free = 0 // where the last label drawn ends, in pixels
  return marks.map((mark, i) => {
    const label = labelOf(mark)
    const x = mark.at * width
    const next = (marks[i + 1]?.at ?? Infinity) * width
    const previous = i > 0 ? marks[i - 1].at * width : 0
    const flip = width - x < labelPx(label)
    const fits = flip ? x - labelPx(label) >= Math.max(free, previous) : x >= free && next - x >= labelPx(label)
    if (!fits) return { mark, flip }
    free = flip ? x : x + labelPx(label)
    return { mark, label, flip }
  })
}
