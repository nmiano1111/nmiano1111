import { cn } from '@/lib/utils'
import type { ButtonHTMLAttributes } from 'react'

type Variant = 'default' | 'ghost' | 'danger' | 'outline'
type Size = 'sm' | 'md'

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant
  size?: Size
}

const variants: Record<Variant, string> = {
  default: 'bg-accent/20 text-accent border border-accent/30 hover:bg-accent/30',
  ghost: 'text-text-subtle hover:text-text hover:bg-base-300/50',
  danger: 'bg-red/10 text-red border border-red/30 hover:bg-red/20',
  outline: 'border border-base-400 text-text-subtle hover:text-text hover:border-base-500',
}

const sizes: Record<Size, string> = {
  sm: 'px-2.5 py-1 text-xs',
  md: 'px-3.5 py-1.5 text-sm',
}

export function Button({
  variant = 'default',
  size = 'md',
  className,
  disabled,
  children,
  ...props
}: ButtonProps) {
  return (
    <button
      {...props}
      disabled={disabled}
      className={cn(
        'inline-flex items-center gap-1.5 rounded font-medium transition-colors',
        'disabled:opacity-40 disabled:cursor-not-allowed',
        variants[variant],
        sizes[size],
        className
      )}
    >
      {children}
    </button>
  )
}
