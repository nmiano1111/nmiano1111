import { cn } from '@/lib/utils'

export function Spinner({ className }: { className?: string }) {
  return (
    <div
      className={cn(
        'w-4 h-4 border-2 border-base-300 border-t-accent rounded-full animate-spin',
        className
      )}
    />
  )
}

export function FullPageSpinner() {
  return (
    <div className="flex items-center justify-center h-64">
      <Spinner className="w-6 h-6" />
    </div>
  )
}
