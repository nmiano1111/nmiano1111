import { Link } from 'react-router-dom'
import { useRuns } from '@/lib/hooks'
import { StatusBadge } from '@/components/ui/StatusBadge'
import { StatCard } from '@/components/ui/StatCard'
import { PageHeader } from '@/components/ui/PageHeader'
import { FullPageSpinner } from '@/components/ui/Spinner'
import { EmptyState } from '@/components/ui/EmptyState'
import { shortId, formatRelativeTime, formatCost, formatDuration, cn } from '@/lib/utils'
import type { Run } from '@/api/types'

function RunRow({ run }: { run: Run }) {
  return (
    <Link
      to={`/runs/${run.id}`}
      className={cn(
        'flex items-center gap-3 px-4 py-2.5 hover:bg-base-300/30 transition-colors',
        'border-b border-base-300/50 last:border-0'
      )}
    >
      <StatusBadge status={run.status} pulse />
      <span className="font-mono text-xs text-accent">{shortId(run.id)}</span>
      <span className="text-sm text-text flex-1 truncate">{
        run.prompt.length > 70 ? run.prompt.slice(0, 68) + '…' : run.prompt
      }</span>
      <span className="text-xs text-text-dim shrink-0">{formatRelativeTime(run.created_at)}</span>
    </Link>
  )
}

export function OverviewPage() {
  const { data: runs = [], isLoading, error } = useRuns(30)

  if (isLoading) return <FullPageSpinner />

  if (error) {
    return (
      <div className="p-6">
        <EmptyState
          icon="⚠"
          title="Cannot reach API server"
          description="Start the dashboard server: agentctl web"
        />
      </div>
    )
  }

  // Compute stats
  const stats = {
    total: runs.length,
    running: runs.filter((r) => r.status === 'running').length,
    completed: runs.filter((r) => r.status === 'completed').length,
    failed: runs.filter((r) => r.status === 'failed').length,
    totalCost: runs.reduce((sum, r) => sum + r.cost_usd, 0),
    totalTokens: runs.reduce((sum, r) => sum + r.input_tokens + r.output_tokens, 0),
    avgDuration: (() => {
      const done = runs.filter((r) => r.duration_ms > 0)
      return done.length
        ? done.reduce((s, r) => s + r.duration_ms, 0) / done.length
        : 0
    })(),
  }

  const recentRuns = [...runs].slice(0, 12)
  const failedRuns = runs.filter((r) => r.status === 'failed').slice(0, 5)

  return (
    <div className="flex flex-col h-full">
      <PageHeader title="Overview" />

      <div className="flex-1 overflow-y-auto p-6 space-y-6">
        {/* Stats row */}
        <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
          <StatCard
            label="Total Runs"
            value={stats.total}
            sub="in database"
          />
          <StatCard
            label="Running Now"
            value={stats.running}
            accent={stats.running > 0 ? 'text-accent' : undefined}
            sub={stats.running > 0 ? 'active' : 'idle'}
          />
          <StatCard
            label="Success Rate"
            value={
              stats.completed + stats.failed > 0
                ? `${Math.round((stats.completed / (stats.completed + stats.failed)) * 100)}%`
                : '—'
            }
            accent={
              stats.completed + stats.failed > 0 &&
              stats.completed / (stats.completed + stats.failed) >= 0.8
                ? 'text-green'
                : 'text-yellow'
            }
            sub={`${stats.completed} completed, ${stats.failed} failed`}
          />
          <StatCard
            label="Total Cost"
            value={formatCost(stats.totalCost)}
            accent="text-text"
            sub={`${stats.totalTokens.toLocaleString()} tokens`}
          />
        </div>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
          {/* Recent runs */}
          <div className="md:col-span-2 card overflow-hidden">
            <div className="px-4 py-3 border-b border-base-300 flex items-center justify-between">
              <span className="text-sm font-medium text-text">Recent Runs</span>
              <Link to="/runs" className="text-xs text-accent hover:underline">
                View all →
              </Link>
            </div>
            {recentRuns.length === 0 ? (
              <EmptyState
                title="No runs yet"
                description='Run agentctl run "your prompt" to get started.'
              />
            ) : (
              <div>
                {recentRuns.map((run) => (
                  <RunRow key={run.id} run={run} />
                ))}
              </div>
            )}
          </div>

          {/* Side panels */}
          <div className="space-y-4">
            {/* Active runs */}
            {stats.running > 0 && (
              <div className="card overflow-hidden">
                <div className="px-4 py-3 border-b border-base-300">
                  <span className="text-sm font-medium text-text">
                    🟢 Active ({stats.running})
                  </span>
                </div>
                {runs
                  .filter((r) => r.status === 'running')
                  .map((run) => (
                    <Link
                      key={run.id}
                      to={`/runs/${run.id}`}
                      className="flex items-center gap-2 px-4 py-2.5 hover:bg-base-300/30 border-b border-base-300/50 last:border-0"
                    >
                      <span className="w-1.5 h-1.5 rounded-full bg-accent animate-pulse shrink-0" />
                      <span className="text-sm text-text truncate flex-1">
                        {run.prompt.length > 40
                          ? run.prompt.slice(0, 38) + '…'
                          : run.prompt}
                      </span>
                    </Link>
                  ))}
              </div>
            )}

            {/* Recent failures */}
            {failedRuns.length > 0 && (
              <div className="card overflow-hidden">
                <div className="px-4 py-3 border-b border-base-300">
                  <span className="text-sm font-medium text-text">
                    ✗ Recent Failures
                  </span>
                </div>
                {failedRuns.map((run) => (
                  <Link
                    key={run.id}
                    to={`/runs/${run.id}`}
                    className="block px-4 py-2.5 hover:bg-base-300/30 border-b border-base-300/50 last:border-0"
                  >
                    <div className="flex items-center gap-2 mb-0.5">
                      <span className="font-mono text-xs text-red">
                        {shortId(run.id)}
                      </span>
                      <span className="text-xs text-text-dim">
                        {formatRelativeTime(run.created_at)}
                      </span>
                    </div>
                    <p className="text-xs text-text-dim truncate">
                      {run.error || run.prompt}
                    </p>
                  </Link>
                ))}
              </div>
            )}

            {/* Quick stats */}
            <div className="card p-4 space-y-3">
              <p className="text-xs font-medium text-text-dim uppercase tracking-wider">
                Performance
              </p>
              <div className="space-y-2">
                <div className="flex justify-between text-sm">
                  <span className="text-text-dim">Avg duration</span>
                  <span className="font-mono text-text">
                    {formatDuration(stats.avgDuration)}
                  </span>
                </div>
                <div className="flex justify-between text-sm">
                  <span className="text-text-dim">Avg cost</span>
                  <span className="font-mono text-text">
                    {stats.completed > 0
                      ? formatCost(stats.totalCost / stats.completed)
                      : '—'}
                  </span>
                </div>
                <div className="flex justify-between text-sm">
                  <span className="text-text-dim">Cancelled</span>
                  <span className="font-mono text-text">
                    {runs.filter((r) => r.status === 'cancelled').length}
                  </span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
