import { Link, useParams } from 'react-router-dom'
import { ArrowLeft, GitBranch } from 'lucide-react'
import { useRun, useRunTree } from '@/lib/hooks'
import { StatusBadge } from '@/components/ui/StatusBadge'
import { PageHeader } from '@/components/ui/PageHeader'
import { FullPageSpinner } from '@/components/ui/Spinner'
import { EmptyState } from '@/components/ui/EmptyState'
import { Button } from '@/components/ui/Button'
import { cn, shortId, formatDuration, formatCost } from '@/lib/utils'
import type { Run } from '@/api/types'

export function RunTreePage() {
  const { id = '' } = useParams()
  const { data: root, isLoading, error } = useRun(id)
  const { data: allRuns = [] } = useRunTree(id)

  if (isLoading) return <FullPageSpinner />

  if (error || !root) {
    return (
      <EmptyState
        icon="⊘"
        title="Run not found"
        description={`No run with ID ${shortId(id)} exists.`}
        action={
          <Link to="/runs">
            <Button variant="default">Back to runs</Button>
          </Link>
        }
      />
    )
  }

  // Build adjacency map: parentID -> children[]
  const childMap = new Map<string, Run[]>()
  for (const run of allRuns) {
    const pid = run.parent_run_id || ''
    if (!childMap.has(pid)) childMap.set(pid, [])
    childMap.get(pid)!.push(run)
  }

  // Total cost across all runs
  const totalCost = allRuns.reduce((s, r) => s + r.cost_usd, 0)
  const totalRuns = allRuns.length

  return (
    <div className="flex flex-col h-full">
      <PageHeader
        title={
          <div className="flex items-center gap-3">
            <Link
              to={`/runs/${id}`}
              className="text-text-dim hover:text-text transition-colors"
            >
              <ArrowLeft size={16} />
            </Link>
            <GitBranch size={14} className="text-accent" />
            <span className="font-mono text-sm text-accent">{shortId(id)}</span>
            <span className="text-text-dim text-sm">— run tree</span>
          </div>
        }
        actions={
          <span className="text-xs text-text-dim">
            {totalRuns} run{totalRuns !== 1 ? 's' : ''}
            {totalCost > 0 && <> · {formatCost(totalCost)} total</>}
          </span>
        }
      />

      <div className="flex-1 overflow-y-auto px-6 py-6">
        <TreeNode
          run={root}
          childMap={childMap}
          depth={0}
          isLast={true}
          prefix=""
        />
      </div>
    </div>
  )
}

interface TreeNodeProps {
  run: Run
  childMap: Map<string, Run[]>
  depth: number
  isLast: boolean
  prefix: string
}

function TreeNode({ run, childMap, depth, isLast, prefix }: TreeNodeProps) {
  const children = childMap.get(run.id) ?? []
  const connector = depth === 0 ? '' : isLast ? '└─ ' : '├─ '
  const childPrefix = depth === 0 ? '' : prefix + (isLast ? '   ' : '│  ')

  return (
    <div>
      {/* Row */}
      <div className="flex items-start gap-2 group">
        {/* Tree lines */}
        {depth > 0 && (
          <span
            className="shrink-0 font-mono text-text-dim/40 text-xs leading-none pt-3 select-none whitespace-pre"
            style={{ letterSpacing: 0 }}
          >
            {prefix}{connector}
          </span>
        )}

        {/* Card */}
        <Link
          to={`/runs/${run.id}`}
          className={cn(
            'flex-1 flex items-center gap-3 px-3 py-2 rounded border border-transparent',
            'hover:border-base-300 hover:bg-base-100/40 transition-colors mb-1',
            depth === 0 && 'border-base-300 bg-base-100/20'
          )}
        >
          <StatusBadge status={run.status} />

          <span className="font-mono text-xs text-accent shrink-0">
            {shortId(run.id)}
          </span>

          {run.agent_id && (
            <span className="text-xs text-text-dim shrink-0 font-mono">
              {run.agent_id}
            </span>
          )}

          <span className="text-xs text-text truncate flex-1 min-w-0">
            {run.prompt}
          </span>

          <span className="text-xs text-text-dim shrink-0 font-mono">
            {formatDuration(run.duration_ms)}
          </span>

          {run.cost_usd > 0 && (
            <span className="text-xs text-text-dim shrink-0 font-mono">
              {formatCost(run.cost_usd)}
            </span>
          )}
        </Link>
      </div>

      {/* Children */}
      {children.map((child, i) => (
        <TreeNode
          key={child.id}
          run={child}
          childMap={childMap}
          depth={depth + 1}
          isLast={i === children.length - 1}
          prefix={childPrefix}
        />
      ))}
    </div>
  )
}
