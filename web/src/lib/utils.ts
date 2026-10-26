import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'
import type { RunStatus, EventKind } from '@/api/types'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function shortId(id: string): string {
  return id.length > 8 ? id.slice(0, 8) : id
}

export function formatDuration(ms: number): string {
  if (ms <= 0) return '—'
  if (ms < 1000) return `${ms}ms`
  const s = ms / 1000
  if (s < 60) return `${s.toFixed(1)}s`
  const m = Math.floor(s / 60)
  const rem = Math.round(s % 60)
  return rem > 0 ? `${m}m ${rem}s` : `${m}m`
}

export function formatCost(usd: number): string {
  if (usd <= 0) return '—'
  return `$${usd.toFixed(4)}`
}

export function formatTokens(input: number, output: number): string {
  const total = input + output
  if (total <= 0) return '—'
  return `${total.toLocaleString()}`
}

export function formatRelativeTime(isoString: string | null): string {
  if (!isoString) return '—'
  const d = Date.now() - new Date(isoString).getTime()
  const s = Math.floor(d / 1000)
  if (s < 60) return `${s}s ago`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m ago`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h ago`
  return `${Math.floor(h / 24)}d ago`
}

export function formatTimestamp(isoString: string | null): string {
  if (!isoString) return '—'
  return new Date(isoString).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

export function formatFullDate(isoString: string | null): string {
  if (!isoString) return '—'
  return new Date(isoString).toLocaleString([], {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

export const STATUS_CONFIG: Record<
  RunStatus,
  { label: string; color: string; dot: string; bg: string }
> = {
  pending: {
    label: 'pending',
    color: 'text-yellow',
    dot: 'bg-yellow',
    bg: 'bg-yellow/10',
  },
  running: {
    label: 'running',
    color: 'text-accent',
    dot: 'bg-accent',
    bg: 'bg-accent/10',
  },
  completed: {
    label: 'done',
    color: 'text-green',
    dot: 'bg-green',
    bg: 'bg-green/10',
  },
  failed: {
    label: 'failed',
    color: 'text-red',
    dot: 'bg-red',
    bg: 'bg-red/10',
  },
  cancelled: {
    label: 'cancelled',
    color: 'text-text-dim',
    dot: 'bg-text-dim',
    bg: 'bg-base-300/50',
  },
}

export const EVENT_CONFIG: Record<
  EventKind,
  { label: string; icon: string; color: string }
> = {
  started:      { label: 'started',     icon: '▶', color: 'text-accent' },
  text:         { label: 'text',        icon: '◆', color: 'text-text' },
  tool_use:     { label: 'tool_use',    icon: '⚙', color: 'text-mauve' },
  tool_result:  { label: 'tool_result', icon: '◇', color: 'text-peach' },
  completed:    { label: 'completed',   icon: '✓', color: 'text-green' },
  failed:       { label: 'failed',      icon: '✗', color: 'text-red' },
  verification: { label: 'verify',      icon: '⊕', color: 'text-sky' },
}
