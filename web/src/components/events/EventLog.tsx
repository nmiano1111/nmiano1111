import { useRef, useEffect } from 'react'
import { cn, EVENT_CONFIG, formatTimestamp } from '@/lib/utils'
import type { AgentEvent, ToolUseData, ToolResultData, CompletedData, FailedData, VerificationData, TextData } from '@/api/types'

interface EventLogProps {
  events: AgentEvent[]
  isLive?: boolean
  className?: string
}

export function EventLog({ events, isLive, className }: EventLogProps) {
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (isLive) {
      bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
    }
  }, [events.length, isLive])

  if (events.length === 0) {
    return (
      <div className={cn('py-8 text-center text-sm text-text-dim', className)}>
        {isLive ? 'Waiting for events…' : 'No events recorded.'}
      </div>
    )
  }

  return (
    <div className={cn('font-mono text-xs space-y-0.5', className)}>
      {events.map((event) => (
        <EventRow key={event.id} event={event} />
      ))}
      {isLive && (
        <div className="flex items-center gap-2 py-1 px-3 text-text-dim">
          <span className="w-1.5 h-1.5 rounded-full bg-accent animate-pulse" />
          <span>live</span>
        </div>
      )}
      <div ref={bottomRef} />
    </div>
  )
}

function EventRow({ event }: { event: AgentEvent }) {
  const cfg = EVENT_CONFIG[event.kind] ?? {
    label: event.kind,
    icon: '·',
    color: 'text-text-dim',
  }

  return (
    <div className="flex items-start gap-3 px-3 py-1 hover:bg-base-300/20 rounded group">
      {/* Timestamp */}
      <span className="text-text-dim shrink-0 w-16 pt-px">
        {formatTimestamp(event.timestamp)}
      </span>

      {/* Kind */}
      <span className={cn('shrink-0 w-24 pt-px', cfg.color)}>
        {cfg.icon} {cfg.label}
      </span>

      {/* Detail */}
      <span className="text-text flex-1 break-all">
        <EventDetail event={event} />
      </span>

      {/* Seq */}
      <span className="text-text-dim shrink-0 opacity-0 group-hover:opacity-100 transition-opacity">
        #{event.seq}
      </span>
    </div>
  )
}

function EventDetail({ event }: { event: AgentEvent }) {
  const data = (event.data ?? {}) as Record<string, unknown>

  switch (event.kind) {
    case 'started':
      return <span className="text-text-dim">pid {(data as {pid?: number}).pid ?? '—'}</span>

    case 'text': {
      const d = data as unknown as TextData
      const content = (d.content ?? '').replace(/\n/g, ' ').trim()
      return (
        <span className="text-text leading-relaxed">
          {content.length > 200 ? content.slice(0, 198) + '…' : content}
        </span>
      )
    }

    case 'tool_use': {
      const d = data as unknown as ToolUseData
      let inputStr = ''
      try {
        inputStr = JSON.stringify(d.input)
        if (inputStr.length > 80) inputStr = inputStr.slice(0, 78) + '…'
      } catch {
        // ignore
      }
      return (
        <span>
          <span className="text-mauve">{d.name}</span>
          {inputStr && (
            <span className="text-text-dim"> → {inputStr}</span>
          )}
        </span>
      )
    }

    case 'tool_result': {
      const d = data as unknown as ToolResultData
      const content = (d.content ?? '').replace(/\n/g, ' ').trim()
      if (d.is_error) {
        return (
          <span className="text-red">
            [error] {content.length > 100 ? content.slice(0, 98) + '…' : content}
          </span>
        )
      }
      return (
        <span className="text-text-dim">
          {content.length > 120 ? content.slice(0, 118) + '…' : content || '(empty)'}
        </span>
      )
    }

    case 'completed': {
      const d = data as unknown as CompletedData
      const parts: string[] = []
      if (d.cost_usd > 0) parts.push(`$${d.cost_usd.toFixed(4)}`)
      if (d.input_tokens + d.output_tokens > 0)
        parts.push(`${(d.input_tokens + d.output_tokens).toLocaleString()} tokens`)
      if (d.duration_ms > 0) parts.push(`${(d.duration_ms / 1000).toFixed(1)}s`)
      return (
        <span>
          {d.result && (
            <span className="text-green mr-2">
              {d.result.length > 80 ? d.result.slice(0, 78) + '…' : d.result}
            </span>
          )}
          {parts.length > 0 && (
            <span className="text-text-dim">({parts.join(' · ')})</span>
          )}
        </span>
      )
    }

    case 'failed': {
      const d = data as unknown as FailedData
      return (
        <span className="text-red">
          {(d.error ?? '').length > 120
            ? d.error.slice(0, 118) + '…'
            : d.error ?? 'unknown error'}
        </span>
      )
    }

    case 'verification': {
      const d = data as unknown as VerificationData
      return (
        <span>
          <span className={d.passed ? 'text-green' : 'text-red'}>
            {d.passed ? '✓ passed' : '✗ failed'}
          </span>
          {d.command && (
            <span className="text-text-dim ml-2">{d.command}</span>
          )}
        </span>
      )
    }

    default:
      return <span className="text-text-dim">{JSON.stringify(data)}</span>
  }
}
