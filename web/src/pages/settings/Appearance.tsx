import { useState } from 'react'
import { SettingsLayout } from '@/components/settings-layout'
import { THEMES, Theme, getTheme, setTheme } from '@/lib/theme'

export default function SettingsAppearance() {
  const [theme, setCurrent] = useState<Theme>(getTheme())

  const choose = (next: Theme) => {
    setTheme(next)
    setCurrent(next)
  }

  return (
    <SettingsLayout currentPath="/settings/appearance">
      <section className="max-w-xl">
        <h2 className="text-[15px] font-bold text-foreground">Theme</h2>
        <p className="mt-1 text-[13px] text-subtle">
          Saved in this browser. Also <span className="text-foreground">:theme gruvbox</span> from any screen.
        </p>
        <div className="pane mt-6 py-2">
          <span className="pane-title">themes</span>
          {THEMES.map((t) => (
            <label key={t.id} className="flex cursor-pointer items-center gap-3 px-4 py-2 text-[13px] hover:bg-raise">
              <input type="radio" name="theme" checked={theme === t.id} onChange={() => choose(t.id)} />
              <span className="w-28 text-foreground">{t.label}</span>
              <span className="text-dim">{t.note}</span>
            </label>
          ))}
        </div>
      </section>
    </SettingsLayout>
  )
}
