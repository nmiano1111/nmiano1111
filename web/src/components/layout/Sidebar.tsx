import { NavLink } from 'react-router-dom'
import { LayoutDashboard, List } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/api/client'

const navItems = [
  { to: '/', icon: LayoutDashboard, label: 'Overview', end: true },
  { to: '/jobs', icon: List, label: 'Jobs', end: false },
]

export function Sidebar() {
  const health = useQuery({
    queryKey: ['health'],
    queryFn: api.health,
    refetchInterval: 10_000,
    retry: 1,
  })

  return (
    <aside className="w-56 shrink-0 flex flex-col border-r border-base-300 bg-base-100 h-full">
      {/* Logo */}
      <div className="h-14 flex items-center px-4 border-b border-base-300">
        <span className="text-lg mr-2">🤖</span>
        <span className="font-semibold text-text tracking-tight">nickbot</span>
      </div>

      {/* Nav */}
      <nav className="flex-1 p-2 space-y-0.5">
        {navItems.map(({ to, icon: Icon, label, end }) => (
          <NavLink
            key={to}
            to={to}
            end={end}
            className={({ isActive }) =>
              cn(
                'flex items-center gap-2.5 px-3 py-2 rounded text-sm transition-colors',
                isActive
                  ? 'bg-accent/10 text-accent font-medium'
                  : 'text-text-subtle hover:text-text hover:bg-base-300/50'
              )
            }
          >
            <Icon size={16} />
            {label}
          </NavLink>
        ))}
      </nav>

      {/* Footer status */}
      <div className="p-3 border-t border-base-300">
        <div className="flex items-center gap-2 text-xs">
          <span
            className={cn(
              'w-1.5 h-1.5 rounded-full',
              health.isSuccess ? 'bg-green' : 'bg-red'
            )}
          />
          <span className="text-text-dim">
            {health.isSuccess
              ? `v${health.data.version}`
              : health.isLoading
                ? 'connecting…'
                : 'offline'}
          </span>
        </div>
      </div>
    </aside>
  )
}
