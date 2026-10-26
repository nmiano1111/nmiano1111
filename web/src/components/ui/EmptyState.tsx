import type { ReactNode } from 'react'

interface EmptyStateProps {
  icon?: ReactNode
  title: string
  description?: string
  action?: ReactNode
}

export function EmptyState({ icon, title, description, action }: EmptyStateProps) {
  return (
    <div className="flex flex-col items-center justify-center py-20 px-6 text-center">
      {icon && (
        <div className="text-4xl mb-4 text-text-dim">{icon}</div>
      )}
      <p className="text-base font-medium text-text mb-1">{title}</p>
      {description && (
        <p className="text-sm text-text-dim max-w-sm mb-4">{description}</p>
      )}
      {action}
    </div>
  )
}
