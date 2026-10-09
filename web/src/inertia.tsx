import { createInertiaApp } from '@inertiajs/react'
import { createRoot } from 'react-dom/client'
import './index.css'

// Lazy load pages to split the bundle and improve initial load time.
// Each page loads on-demand when navigated to.
const pages = import.meta.glob<{ default: React.ComponentType<any> }>(
  './pages/**/*.tsx',
  { eager: false }
)

// Map file paths to page component names and resolve them lazily
const resolvePageComponent = async (name: string) => {
  // Convert page names like 'Home' to './pages/Home.tsx'
  // or 'SettingsLicense' to './pages/settings/License.tsx'
  let path = ''

  // Handle nested pages (Settings*)
  if (name.startsWith('Settings')) {
    const settingsName = name.slice(8) // Remove 'Settings' prefix
    path = `./pages/settings/${settingsName}.tsx`
  } else {
    path = `./pages/${name}.tsx`
  }

  const page = pages[path]
  if (!page) {
    console.error(`Page ${name} not found at ${path}`)
    // Fallback to Home if page not found
    return (await pages['./pages/Home.tsx']()).default
  }

  return (await page()).default
}

// Track if app has been initialized
let appInitialized = false

createInertiaApp({
  resolve: (name) => resolvePageComponent(name),
  setup({ el, App, props }) {
    if (appInitialized) {
      console.warn('Inertia app already initialized, skipping duplicate setup')
      return
    }
    appInitialized = true
    createRoot(el).render(<App {...props} />)
  },
})
