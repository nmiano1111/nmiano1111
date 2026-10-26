import { cn } from '@/lib/utils'
import type { ReactNode } from 'react'

interface PageHeaderProps {
  title: ReactNode
  subtitle?: ReactNode
  actions?: ReactNode
  className?: string
}

export function PageHeader({ title, subtitle, actions, className }: PageHeaderProps) {
  return (
    <div
      className={cn(
        'h-14 flex items-center justify-between px-6 border-b border-base-300 shrink-0',
        className
      )}
    >
      <div className="flex items-baseline gap-3">
        <h1 className="text-base font-semibold text-text">{title}</h1>
        {subtitle && (
          <span className="text-sm text-text-dim">{subtitle}</span>
        )}
      </div>
      {actions && <div className="flex items-center gap-2">{actions}</div>}
    </div>
  )
}
