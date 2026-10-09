// Deploys on a time chart: a dashed line where each release first started,
// with its short version when few enough fit. Place it inside the chart's
// bars, in a relative box; it ignores the mouse so the bars keep their hover.

export interface Release {
  version: string
  first_seen: string
}

// A git SHA as its first 7 characters; any other release name as it is.
export function shortRelease(release: string): string {
  return /^[0-9a-f]{12,}$/i.test(release) ? release.slice(0, 7) : release
}

export function ReleaseMarks({ releases, from, to }: { releases?: Release[] | null; from: number; to: number }) {
  const span = to - from
  if (!releases?.length || span <= 0) return null
  const shown = releases
    .map((r) => ({ ...r, at: (new Date(r.first_seen).getTime() - from) / span }))
    .filter((r) => r.at >= 0 && r.at <= 1)
  const labeled = shown.length <= 6

  return (
    <div className="pointer-events-none absolute inset-0" aria-hidden="true">
      {shown.map((r) => (
        <div key={r.version} className="absolute bottom-0 top-0 border-l border-dashed border-subtle" style={{ left: `${r.at * 100}%` }}>
          {labeled && (
            <span className="absolute -top-3.5 left-1 whitespace-nowrap bg-background px-0.5 text-[10px] leading-none text-subtle">
              {shortRelease(r.version)}
            </span>
          )}
        </div>
      ))}
    </div>
  )
}
