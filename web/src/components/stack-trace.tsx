import { useState } from 'react'
import { parseStack, foldLibraryFrames } from '@/lib/stack'

// Your app's frames first; library and runtime frames fold into one line each.
export function StackTrace({ trace, errorFile }: { trace: string; errorFile?: string }) {
  const frames = parseStack(trace)
  const appFrames = frames.filter((f) => !f.library).length
  const libraryFrames = frames.length - appFrames
  const [all, setAll] = useState(appFrames === 0)
  const lines = all ? frames.map((frame) => ({ kind: 'frame' as const, frame })) : foldLibraryFrames(frames)
  const isErrorFrame = (text: string) => Boolean(errorFile && text.includes(errorFile))

  return (
    <div className="pane mt-7 px-4 pb-4 pt-6">
      <span className="pane-title">stack trace</span>
      {libraryFrames > 0 && appFrames > 0 && (
        <span className="pane-right">
          {all ? 'all frames' : 'app frames'} ·{' '}
          <button onClick={() => setAll(!all)} className="text-foreground hover:underline">
            {all ? 'app frames only' : 'show all'}
          </button>
        </span>
      )}
      <div className="overflow-x-auto text-[13px]">
        {lines.map((line, i) =>
          line.kind === 'folded' ? (
            <div key={`f${i}`} className="grid grid-cols-[4ch_1fr] py-0.5 text-dim">
              <span />
              <button onClick={() => setAll(true)} className="text-left hover:text-foreground">
                … {line.count} framework frame{line.count !== 1 ? 's' : ''}
              </button>
            </div>
          ) : (
            <div key={line.frame.n} className="grid grid-cols-[4ch_1fr] whitespace-pre py-0.5">
              <span className="text-dim">{line.frame.n}</span>
              <span className={line.frame.library ? 'text-dim' : isErrorFrame(line.frame.text) ? 'font-bold text-foreground' : 'text-text'}>
                {line.frame.text}
              </span>
            </div>
          ),
        )}
      </div>
    </div>
  )
}
