import { cn } from '@/lib/utils'
import type { ReactNode } from 'react'

interface StatCardProps {
  label: string
  value: ReactNode
  sub?: ReactNode
  accent?: string
  className?: string
}

export function StatCard({ label, value, sub, accent, className }: StatCardProps) {
  return (
    <div className={cn('card p-4', className)}>
      <p className="field-label mb-1">{label}</p>
      <p className={cn('text-2xl font-semibold tracking-tight', accent ?? 'text-text')}>
        {value}
      </p>
      {sub && <p className="text-xs text-text-dim mt-1">{sub}</p>}
    </div>
  )
}
