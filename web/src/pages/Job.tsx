import { Link, useParams } from 'react-router-dom'
import { ArrowLeft, GitBranch } from 'lucide-react'
import { useJob } from '@/lib/hooks'
import { StatusBadge } from '@/components/ui/StatusBadge'
import { PageHeader } from '@/components/ui/PageHeader'
import { FullPageSpinner } from '@/components/ui/Spinner'
import { EmptyState } from '@/components/ui/EmptyState'
import { Button } from '@/components/ui/Button'
import { RunGraph } from '@/components/graph/RunGraph'
import { cn, shortId, formatTokens, formatFullDate, formatCost, formatDuration } from '@/lib/utils'

export function JobPage() {
  const { id = '' } = useParams()
  const { data: job, isLoading, error } = useJob(id)

  if (isLoading) return <FullPageSpinner />

  if (error || !job) {
    return (
      <EmptyState
        icon="⊘"
        title="Job not found"
        description={`No job with ID ${shortId(id)} exists.`}
        action={
          <Link to="/jobs">
            <Button variant="default">Back to jobs</Button>
          </Link>
        }
      />
    )
  }

  const totalTokens = job.runs.reduce(
    (s, r) => ({ in: s.in + r.input_tokens, out: s.out + r.output_tokens }),
    { in: 0, out: 0 }
  )

  return (
    <div className="flex flex-col h-full">
      <PageHeader
        title={
          <div className="flex items-center gap-3">
            <Link to="/jobs" className="text-text-dim hover:text-text transition-colors">
              <ArrowLeft size={16} />
            </Link>
            <GitBranch size={14} className="text-accent" />
            <span className="font-mono text-sm text-accent">{shortId(id)}</span>
            <StatusBadge status={job.status} pulse />
          </div>
        }
        actions={
          <span className="text-xs text-text-dim">
            {job.run_count} run{job.run_count !== 1 ? 's' : ''}
          </span>
        }
      />

      <div className="flex-1 overflow-y-auto">
        {/* Aggregated stats */}
        <div className="px-6 py-4 border-b border-base-300 bg-base-100/30">
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            <MetaField label="Total cost"    value={formatCost(job.total_cost)} mono />
            <MetaField label="Total tokens"  value={formatTokens(totalTokens.in, totalTokens.out)} mono />
            <MetaField label="Root duration" value={formatDuration(job.duration_ms)} mono />
            <MetaField label="Created"       value={formatFullDate(job.created_at)} />
          </div>
        </div>

        {/* Prompt */}
        <div className="px-6 py-4 border-b border-base-300">
          <p className="field-label mb-2">Prompt</p>
          <p className="text-sm text-text leading-relaxed whitespace-pre-wrap">{job.prompt}</p>
        </div>

        {/* Result */}
        {job.result && (
          <div className="px-6 py-4 border-b border-base-300">
            <p className="field-label mb-2">Result</p>
            <p className="text-sm text-text leading-relaxed whitespace-pre-wrap">{job.result}</p>
          </div>
        )}

        {/* Visual DAG */}
        <RunGraph
          runs={job.runs}
          className="h-[420px] mx-6 my-4 rounded border border-base-300 overflow-hidden"
        />
      </div>
    </div>
  )
}

function MetaField({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div>
      <p className="field-label mb-0.5">{label}</p>
      <p className={cn('text-sm text-text truncate', mono && 'font-mono')} title={value}>
        {value}
      </p>
    </div>
  )
}
