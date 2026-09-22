// Stack traces arrive as one raw string (context.stack_trace). The issue page
// shows your app's frames first and folds library and runtime frames.

// Library and runtime code: gems, node_modules, vendored code, the Go module
// cache and standard library, Node internals, Python site-packages.
const LIBRARY = [
  /\/gems\//,
  /\/bundle\//,
  /\/vendor\//,
  /node_modules\//,
  /\/go\/pkg\/mod\//,
  /\/usr\/local\/go\//,
  /\/lib\/ruby\//,
  /site-packages\//,
  /dist-packages\//,
  /<internal:/,
  /\bnode:internal\b/,
  /\(internal\//,
  /webpack-internal:/,
]

export interface Frame {
  n: number
  text: string
  library: boolean
}

export const isLibraryFrame = (line: string) => LIBRARY.some((re) => re.test(line))

export function parseStack(trace: string): Frame[] {
  return trace
    .split('\n')
    .map((line) => line.replace(/\s+$/, ''))
    .filter(Boolean)
    .map((text, i) => ({ n: i + 1, text: text.trim(), library: isLibraryFrame(text) }))
}

// App frames stay; each run of library frames becomes one "… N framework frames" line.
export type StackLine = { kind: 'frame'; frame: Frame } | { kind: 'folded'; count: number }

export function foldLibraryFrames(frames: Frame[]): StackLine[] {
  const out: StackLine[] = []
  for (const frame of frames) {
    const last = out[out.length - 1]
    if (!frame.library) out.push({ kind: 'frame', frame })
    else if (last?.kind === 'folded') last.count++
    else out.push({ kind: 'folded', count: 1 })
  }
  return out
}
