import { useState } from 'react'
import { useParams, Link, useNavigate } from 'react-router-dom'
import { ArrowLeft, StopCircle, Activity, GitBranch, List, Network, Workflow } from 'lucide-react'
import { useRun, useEvents, useLiveRun, useCancelRun, useChildRuns } from '@/lib/hooks'
import { StatusBadge } from '@/components/ui/StatusBadge'
import { Button } from '@/components/ui/Button'
import { PageHeader } from '@/components/ui/PageHeader'
import { FullPageSpinner } from '@/components/ui/Spinner'
import { EmptyState } from '@/components/ui/EmptyState'
import { EventLog } from '@/components/events/EventLog'
import { EventGraph } from '@/components/graph/EventGraph'
import {
  cn,
  shortId,
  formatDuration,
  formatCost,
  formatTokens,
  formatFullDate,
  formatRelativeTime,
} from '@/lib/utils'
type Tab = 'log' | 'graph'

export function RunDetailPage() {
  const { id = '' } = useParams()
  const navigate = useNavigate()
  const [tab, setTab] = useState<Tab>('log')

  const { data: initialRun, isLoading, error } = useRun(id)
  const { data: initialEvents = [] } = useEvents(id)
  const { data: childRuns = [] } = useChildRuns(id)
  const cancelRun = useCancelRun()

  // Live stream (replays existing events + streams new ones)
  const live = useLiveRun(id, initialRun ?? null)

  const run = live.run ?? initialRun
  const events = live.events.length > 0 ? live.events : initialEvents
  const isRunning = run?.status === 'running' || run?.status === 'pending'

  if (isLoading) return <FullPageSpinner />

  if (error || !run) {
    return (
      <EmptyState
        icon="⊘"
        title="Run not found"
        description={`No run with ID ${shortId(id)} exists in the database.`}
        action={
          <Link to="/jobs">
            <Button variant="default">Back to jobs</Button>
          </Link>
        }
      />
    )
  }

  const handleCancel = () => {
    if (window.confirm(`Cancel run ${shortId(run.id)}?`)) {
      cancelRun.mutate(run.id)
    }
  }

  return (
    <div className="flex flex-col h-full">
      <PageHeader
        title={
          <div className="flex items-center gap-3">
            <button
              onClick={() => navigate(-1)}
              className="text-text-dim hover:text-text transition-colors"
            >
              <ArrowLeft size={16} />
            </button>
            <span className="font-mono text-sm text-accent">{shortId(run.id)}</span>
            <StatusBadge status={run.status} pulse />
          </div>
        }
        actions={
          isRunning && (
            <Button
              variant="danger"
              size="sm"
              onClick={handleCancel}
              disabled={cancelRun.isPending}
            >
              <StopCircle size={13} />
              {cancelRun.isPending ? 'Cancelling…' : 'Cancel'}
            </Button>
          )
        }
      />

      <div className="flex-1 overflow-y-auto">
        {/* Metadata strip */}
        <div className="px-6 py-4 border-b border-base-300 bg-base-100/30">
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-4">
            <MetaField label="Agent" value={run.agent_id || '—'} />
            <MetaField label="Runner" value={run.runner_name || '—'} />
            <MetaField label="Model" value={run.model || '—'} />
            <MetaField label="Cost" value={formatCost(run.cost_usd)} mono />
          </div>
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            <MetaField label="Duration" value={formatDuration(run.duration_ms)} mono />
            <MetaField
              label="Tokens"
              value={formatTokens(run.input_tokens, run.output_tokens)}
              mono
            />
            <MetaField label="Created" value={formatFullDate(run.created_at)} />
            <MetaField label="Completed" value={formatFullDate(run.completed_at)} />
          </div>
        </div>

        {/* Prompt */}
        <div className="px-6 py-4 border-b border-base-300">
          <p className="field-label mb-2">Prompt</p>
          <p className="text-sm text-text leading-relaxed whitespace-pre-wrap">
            {run.prompt}
          </p>
        </div>

        {/* Result / Error */}
        {run.result && (
          <div className="px-6 py-4 border-b border-base-300">
            <p className="field-label mb-2">Result</p>
            <p className="text-sm text-text leading-relaxed whitespace-pre-wrap">
              {run.result}
            </p>
          </div>
        )}

        {run.error && (
          <div className="px-6 py-4 border-b border-base-300">
            <p className="field-label mb-2 text-red">Error</p>
            <p className="text-sm text-red/80 font-mono whitespace-pre-wrap">
              {run.error}
            </p>
          </div>
        )}

        {/* Job context — shown for any run that belongs to a multi-run job */}
        {run.job_id && run.job_id !== run.id && (
          <div className="px-6 py-3 border-b border-base-300 bg-accent/5">
            <span className="text-xs text-text-dim flex items-center gap-2">
              <GitBranch size={11} className="text-accent shrink-0" />
              Part of job{' '}
              <Link
                to={`/jobs/${run.job_id}`}
                className="text-accent hover:underline font-mono"
              >
                {shortId(run.job_id)}
              </Link>
              {run.parent_run_id && (
                <>
                  <span className="text-text-dim/40">·</span>
                  spawned by{' '}
                  <Link
                    to={`/runs/${run.parent_run_id}`}
                    className="text-accent hover:underline font-mono"
                  >
                    {shortId(run.parent_run_id)}
                  </Link>
                </>
              )}
            </span>
          </div>
        )}

        {/* Delegated tasks (child runs) */}
        {childRuns.length > 0 && (
          <div className="px-6 py-4 border-b border-base-300">
            <div className="flex items-center justify-between mb-3">
              <p className="field-label flex items-center gap-1.5">
                <Network size={12} />
                Delegated Tasks ({childRuns.length})
              </p>
              <Link
                to={`/runs/${run.id}/tree`}
                className="flex items-center gap-1 text-xs text-accent hover:underline"
              >
                <Workflow size={11} />
                View full tree
              </Link>
            </div>
            <div className="space-y-1.5">
              {childRuns.map((child) => (
                <Link
                  key={child.id}
                  to={`/runs/${child.id}`}
                  className="flex items-center gap-3 px-3 py-2 rounded bg-base-100/40 hover:bg-base-100/80 transition-colors group"
                >
                  <StatusBadge status={child.status} />
                  <span className="font-mono text-xs text-accent group-hover:underline">
                    {shortId(child.id)}
                  </span>
                  <span className="text-xs text-text-dim truncate flex-1">
                    {child.prompt}
                  </span>
                  <span className="text-xs text-text-dim shrink-0">
                    {formatRelativeTime(child.created_at)}
                  </span>
                  {child.cost_usd > 0 && (
                    <span className="text-xs text-text-dim font-mono shrink-0">
                      {formatCost(child.cost_usd)}
                    </span>
                  )}
                </Link>
              ))}
            </div>
          </div>
        )}

        {/* Tab bar */}
        <div className="flex items-center gap-0 border-b border-base-300 px-6">
          <TabButton
            active={tab === 'log'}
            onClick={() => setTab('log')}
            icon={<List size={13} />}
            label={`Events (${events.length})`}
          />
          <TabButton
            active={tab === 'graph'}
            onClick={() => setTab('graph')}
            icon={<Activity size={13} />}
            label="Graph"
          />
        </div>

        {/* Tab content */}
        {tab === 'log' && (
          <div className="py-3">
            <EventLog
              events={events}
              isLive={isRunning && !live.isDone}
            />
            {live.error && (
              <p className="px-6 py-2 text-xs text-red">
                Stream error: {live.error}
              </p>
            )}
          </div>
        )}

        {tab === 'graph' && (
          <EventGraph
            events={events}
            className="h-[520px] mx-6 my-4 rounded border border-base-300 overflow-hidden"
          />
        )}
      </div>
    </div>
  )
}

function MetaField({
  label,
  value,
  mono,
}: {
  label: string
  value: string
  mono?: boolean
}) {
  return (
    <div>
      <p className="field-label mb-0.5">{label}</p>
      <p
        className={cn(
          'text-sm text-text truncate',
          mono && 'font-mono'
        )}
        title={value}
      >
        {value}
      </p>
    </div>
  )
}

function TabButton({
  active,
  onClick,
  icon,
  label,
}: {
  active: boolean
  onClick: () => void
  icon: React.ReactNode
  label: string
}) {
  return (
    <button
      onClick={onClick}
      className={cn(
        'flex items-center gap-1.5 px-4 py-3 text-sm transition-colors border-b-2',
        active
          ? 'text-accent border-accent'
          : 'text-text-dim border-transparent hover:text-text'
      )}
    >
      {icon}
      {label}
    </button>
  )
}
