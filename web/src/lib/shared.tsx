// Shared types and components for LogNorth
// See docs/EVENT_MODEL.md for the full specification

import { AlertTriangle, CheckCircle } from 'lucide-react'
import { ReactNode } from 'react'
import type { UptimeState } from '@/lib/uptime'

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
    <div className="mb-8 flex flex-wrap items-end justify-between gap-4">
      <div>
        <h1 className="text-[26px] font-bold leading-tight tracking-tight text-foreground">{title}</h1>
        {subtitle && <p className="mt-1 text-[13px] text-subtle">{subtitle}</p>}
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
    <div className="border border-dashed border-border py-16 text-center">
      <p className="mb-1 font-bold text-foreground">{title}</p>
      {description && <p className="text-sm text-subtle">{description}</p>}
      {children}
    </div>
  )
}

// List container with border and divide
// === Types ===

export interface Application {
  id: number
  name: string
  stats?: {
    total: number
    errors: number
  }
  uptime?: UptimeState // the URL ping, when the app has one
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
  daily?: number[] // Errors per UTC day, last 14 days, oldest first
}

// === Error Config ===
// Binary: error or not. No levels.

export const errorConfig = {
  error: {
    icon: AlertTriangle,
    colors: 'text-[#e11d48] bg-[#e11d48]/10',
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

// === Helper to parse context ===

// Fields the pages render as text. SDKs send whatever they like, and an object
// rendered as a React child crashes the page, so these become JSON strings.
const TEXT_FIELDS = [
  'method', 'path', 'controller', 'action', 'route', 'handler', 'job', 'error', 'environment', 'user', 'ip',
  'user_agent', 'body', 'error_type', 'error_location', 'error_class', 'error_caller', 'error_file', 'stack_trace',
]

const asText = (v: unknown) => (v !== null && typeof v === 'object' ? JSON.stringify(v) : v)

export const parseContext = (contextStr?: string): LogContext => {
  if (!contextStr) return {}
  try {
    const parsed = JSON.parse(contextStr)
    // Ensure we always return an object, even if context is "null" or invalid
    if (!parsed || typeof parsed !== 'object') return {}
    for (const field of TEXT_FIELDS) {
      if (field in parsed) parsed[field] = asText(parsed[field])
    }
    if (parsed.headers && typeof parsed.headers === 'object') {
      for (const key of Object.keys(parsed.headers)) parsed.headers[key] = asText(parsed.headers[key])
    }
    return parsed
  } catch {
    return {}
  }
}
