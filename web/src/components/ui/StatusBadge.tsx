import { cn, STATUS_CONFIG } from '@/lib/utils'
import type { RunStatus } from '@/api/types'

interface StatusBadgeProps {
  status: RunStatus
  pulse?: boolean
  className?: string
}

export function StatusBadge({ status, pulse, className }: StatusBadgeProps) {
  const cfg = STATUS_CONFIG[status]
  return (
    <span
      className={cn(
        'status-badge',
        cfg.color,
        cfg.bg,
        className
      )}
    >
      <span
        className={cn(
          'w-1.5 h-1.5 rounded-full shrink-0',
          cfg.dot,
          pulse && status === 'running' && 'animate-pulse'
        )}
      />
      {cfg.label}
    </span>
  )
}
