import { createInertiaApp } from '@inertiajs/react'
import { createRoot } from 'react-dom/client'
import './index.css'

// Import LogNorth pages
import Setup from './pages/Setup'
import Home from './pages/Home'
import Login from './pages/Login'
import Onboarding from './pages/Onboarding'
import Log from './pages/Log'
import LogDetail from './pages/LogDetail'
import Issues from './pages/Issues'
import Settings from './pages/Settings'
import Endpoints from './pages/Endpoints'

// Map of page components
const pages: Record<string, React.ComponentType<any>> = {
  Setup,
  Home,
  Login,
  Onboarding,
  Log,
  LogDetail,
  Issues,
  Settings,
  Endpoints,
}

// Track if app has been initialized
let appInitialized = false

createInertiaApp({
  resolve: (name) => {
    const page = pages[name]
    if (!page) {
      console.error(`Page ${name} not found in Inertia page registry`)
      return pages['Home']
    }
    return page
  },
  setup({ el, App, props }) {
    if (appInitialized) {
      console.warn('Inertia app already initialized, skipping duplicate setup')
      return
    }
    appInitialized = true
    createRoot(el).render(<App {...props} />)
  },
})
