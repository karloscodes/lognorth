// Shared types and components for LogNorth
// See docs/EVENT_MODEL.md for the full specification

import { AlertTriangle, CheckCircle, Inbox } from 'lucide-react'
import { ReactNode } from 'react'

// === UI Components ===

// Monospace number display - use for counts, stats, durations
export function Mono({ children, className = '', title }: { children: ReactNode; className?: string; title?: string }) {
  return <span className={`font-mono tabular-nums ${className}`} title={title}>{children}</span>
}

// Code/path display - use for paths, error messages, technical strings
export function Code({ children, className = '' }: { children: ReactNode; className?: string }) {
  return <span className={`font-mono ${className}`}>{children}</span>
}

// Page header with title and optional subtitle
export function PageHeader({
  title,
  subtitle,
  children
}: {
  title: ReactNode
  subtitle?: ReactNode
  children?: ReactNode
}) {
  return (
    <div className="flex items-center justify-between mb-8">
      <div>
        <h1 className="text-2xl font-bold text-foreground">{title}</h1>
        {subtitle && <p className="text-sm text-foreground/50 mt-1">{subtitle}</p>}
      </div>
      {children}
    </div>
  )
}

// Empty state for lists
export function EmptyState({
  title,
  description,
  children
}: {
  title: string
  description?: string
  children?: ReactNode
}) {
  return (
    <div className="border border-foreground/10 py-16 text-center">
      <Inbox className="w-10 h-10 text-foreground/20 mx-auto mb-4" />
      <p className="font-semibold text-foreground mb-1">{title}</p>
      {description && <p className="text-sm text-foreground/50">{description}</p>}
      {children}
    </div>
  )
}

// List container with border and divide
export function ListContainer({ children }: { children: ReactNode }) {
  return (
    <div className="bg-card border border-foreground/10 divide-y divide-foreground/10 overflow-hidden">
      {children}
    </div>
  )
}

// === Types ===

export interface Application {
  id: number
  name: string
  stats?: {
    total: number
    errors: number
  }
}

// LogEntry represents a unified log entry (HTTP request, job, or generic log)
export interface LogEntry {
  id: number
  api_key_id: number
  timestamp: string
  message: string
  trace_id?: string
  duration_ms: number
  is_error: boolean
  context?: string // JSON string with LogContext
  created_at: string
}

// LogContext represents the parsed context field
export interface LogContext {
  // HTTP request fields
  method?: string
  path?: string
  status?: number
  // Route enrichment from SDKs
  controller?: string
  action?: string
  route?: string
  handler?: string
  // Job fields
  job?: string
  // Common fields
  error?: string
  environment?: string
  user?: string
  ip?: string
  user_agent?: string
  headers?: Record<string, string>
  body?: string
  // Error grouping fields (SDK-extracted)
  error_type?: string
  error_location?: string
  // Structured error fields (new SDKs)
  error_class?: string
  error_caller?: string
  error_file?: string
  error_line?: number
  stack_trace?: string
  // Custom fields
  extra?: Record<string, any>
}

export interface OccurrencePoint {
  hour: string
  count: number
}

export interface IssueDetail extends Issue {
  api_key_id: number
  entries: LogEntry[]
}

export interface RelatedLocation {
  file: string
  line: number
  count: number
  hash: string
  same_file: boolean // hash_2 match (same file) vs hash_1 match (different file)
}

export interface Issue {
  error: string
  error_class?: string
  error_file?: string
  error_line?: number
  hash: string
  api_key_id: number
  count: number
  first_seen: string
  last_seen: string
  paths: string[]
  unseen: boolean  // New occurrences since last viewed
  active: boolean  // Had occurrences in last 24h
  muted: boolean   // User muted this issue
  is_new: boolean  // First seen < 24h ago
  trend: 'up' | 'down' | 'stable'  // Trend direction
}

// === Error Config ===
// Binary: error or not. No levels.

export const errorConfig = {
  error: {
    icon: AlertTriangle,
    colors: 'text-[#e11d48] bg-rose-50',
    label: 'Error',
  },
  success: {
    icon: CheckCircle,
    colors: 'text-foreground/50 bg-foreground/[0.02]',
    label: 'Success',
  },
} as const

export const getConfig = (isError: boolean) =>
  isError ? errorConfig.error : errorConfig.success

// === Status Colors ===

export const statusColor = (status?: number) => {
  if (!status) return 'text-foreground/50'
  if (status >= 500) return 'text-[#e11d48] font-semibold'
  if (status >= 400) return 'text-foreground'
  return 'text-foreground/50'
}

// === HTTP Method Colors ===

// Methods have no color decoration - status codes carry the semantic weight
export const methodColor = (_method?: string) => 'text-foreground/50'

// === Helper to parse context ===

export const parseContext = (contextStr?: string): LogContext => {
  if (!contextStr) return {}
  try {
    const parsed = JSON.parse(contextStr)
    // Ensure we always return an object, even if context is "null" or invalid
    return parsed && typeof parsed === 'object' ? parsed : {}
  } catch {
    return {}
  }
}
