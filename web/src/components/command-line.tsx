import { useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { completions, history, runCommand, type CommandContext } from '@/lib/commands'

// The : line in the status bar. Enter runs, Tab completes and cycles, ↑ ↓
// walk the history, Esc or backspace on an empty line leaves.
export function CommandLine({ ctx, onClose: close }: { ctx: CommandContext; onClose: (message?: string) => void }) {
  // Enter closes the line, then the input's blur would close it again and
  // wipe the message, so only the first close counts.
  const closed = useRef(false)
  const onClose = (message?: string) => {
    if (closed.current) return
    closed.current = true
    close(message)
  }
  const [value, setValue] = useState('')
  const [wild, setWild] = useState<{ base: string; options: string[]; index: number } | null>(null)
  const [back, setBack] = useState(history.length) // position in the history
  const input = useRef<HTMLInputElement>(null)

  useEffect(() => input.current?.focus(), [])

  const complete = (step: number) => {
    if (wild && wild.options.length > 1) {
      const index = (wild.index + step + wild.options.length) % wild.options.length
      setWild({ ...wild, index })
      setValue(wild.base + wild.options[index])
      return
    }
    const { base, options } = completions(value, ctx)
    if (!options.length) return
    setValue(base + options[0])
    setWild(options.length > 1 ? { base, options, index: 0 } : null)
  }

  const recall = (to: number) => {
    const at = Math.max(0, Math.min(history.length, to))
    setBack(at)
    setValue(history[at] ?? '')
    setWild(null)
  }

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    e.stopPropagation() // the screen's own keys stay quiet while you type
    switch (e.key) {
      case 'Escape':
        e.preventDefault()
        return onClose()
      case 'Enter': {
        e.preventDefault()
        if (value.trim() && history[history.length - 1] !== value) history.push(value)
        return onClose(runCommand(value, ctx) || undefined)
      }
      case 'Tab':
        e.preventDefault()
        return complete(e.shiftKey ? -1 : 1)
      case 'ArrowUp':
        e.preventDefault()
        return recall(back - 1)
      case 'ArrowDown':
        e.preventDefault()
        return recall(back + 1)
      case 'Backspace':
        if (!value) {
          e.preventDefault()
          onClose()
        }
    }
  }

  return (
    <>
      <span className="flex items-center bg-accent px-3 font-bold uppercase text-accent-foreground">command</span>
      <label className="flex min-w-0 flex-1 items-center px-3 text-foreground">
        :
        <input
          ref={input}
          value={value}
          onChange={(e) => {
            setValue(e.target.value)
            setWild(null)
          }}
          onKeyDown={onKeyDown}
          onBlur={() => onClose()}
          aria-label="Command"
          spellCheck={false}
          autoComplete="off"
          className="min-w-0 flex-1 bg-transparent font-[inherit] text-foreground caret-accent outline-none"
        />
      </label>
      {wild && (
        <span className="flex items-center gap-2 overflow-hidden px-3" aria-label="Completions">
          {wild.options.slice(0, 10).map((o, i) => (
            <span key={o} className={i === wild.index ? 'bg-foreground px-1 text-background' : 'text-subtle'}>
              {o}
            </span>
          ))}
        </span>
      )}
    </>
  )
}
