import { useState, useMemo } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  useReactTable,
  getCoreRowModel,
  getSortedRowModel,
  getFilteredRowModel,
  flexRender,
  createColumnHelper,
  type SortingState,
  type ColumnFiltersState,
} from '@tanstack/react-table'
import { ArrowUpDown, ArrowUp, ArrowDown, RefreshCw, Search, X, GitBranch } from 'lucide-react'
import { useJobs } from '@/lib/hooks'
import { StatusBadge } from '@/components/ui/StatusBadge'
import { PageHeader } from '@/components/ui/PageHeader'
import { Button } from '@/components/ui/Button'
import { EmptyState } from '@/components/ui/EmptyState'
import { FullPageSpinner } from '@/components/ui/Spinner'
import {
  cn,
  shortId,
  formatRelativeTime,
  formatCost,
  formatDuration,
} from '@/lib/utils'
import type { JobSummary, RunStatus } from '@/api/types'

const col = createColumnHelper<JobSummary>()

const STATUS_OPTIONS: { value: RunStatus | 'all'; label: string }[] = [
  { value: 'all', label: 'All statuses' },
  { value: 'running', label: 'Running' },
  { value: 'completed', label: 'Completed' },
  { value: 'failed', label: 'Failed' },
  { value: 'cancelled', label: 'Cancelled' },
  { value: 'pending', label: 'Pending' },
]

const columns = [
  col.accessor('status', {
    header: 'Status',
    cell: (info) => <StatusBadge status={info.getValue()} pulse />,
    enableSorting: false,
  }),
  col.accessor('id', {
    header: 'ID',
    cell: (info) => (
      <span className="font-mono text-accent text-xs font-medium">
        {shortId(info.getValue())}
      </span>
    ),
    enableSorting: false,
  }),
  col.accessor('agent_id', {
    header: 'Agent',
    cell: (info) => (
      <span className="text-xs text-text-dim truncate max-w-[120px] block">
        {info.getValue() || '—'}
      </span>
    ),
  }),
  col.accessor('prompt', {
    header: 'Prompt',
    cell: (info) => {
      const val = info.getValue()
      return (
        <span className="text-sm text-text truncate max-w-[340px] block" title={val}>
          {val.length > 80 ? val.slice(0, 78) + '…' : val}
        </span>
      )
    },
  }),
  col.accessor('run_count', {
    header: 'Runs',
    cell: (info) => {
      const count = info.getValue()
      if (count <= 1) return <span className="text-xs text-text-dim">—</span>
      return (
        <span className="inline-flex items-center gap-1 text-xs text-text-dim">
          <GitBranch size={10} />
          {count}
        </span>
      )
    },
  }),
  col.accessor('total_cost', {
    header: 'Cost',
    cell: (info) => (
      <span className="text-xs text-text-dim font-mono">
        {formatCost(info.getValue())}
      </span>
    ),
  }),
  col.accessor('duration_ms', {
    header: 'Duration',
    cell: (info) => (
      <span className="text-xs text-text-dim font-mono">
        {formatDuration(info.getValue())}
      </span>
    ),
  }),
  col.accessor('created_at', {
    header: 'Created',
    cell: (info) => (
      <span className="text-xs text-text-dim whitespace-nowrap">
        {formatRelativeTime(info.getValue())}
      </span>
    ),
    sortingFn: 'datetime',
  }),
]

export function JobsPage() {
  const navigate = useNavigate()
  const { data: jobs = [], isLoading, error, refetch, isFetching } = useJobs(100)

  const [sorting, setSorting] = useState<SortingState>([
    { id: 'created_at', desc: true },
  ])
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])
  const [globalFilter, setGlobalFilter] = useState('')
  const [statusFilter, setStatusFilter] = useState<RunStatus | 'all'>('all')

  const filteredJobs = useMemo(() => {
    if (statusFilter === 'all') return jobs
    return jobs.filter((j) => j.status === statusFilter)
  }, [jobs, statusFilter])

  const table = useReactTable({
    data: filteredJobs,
    columns,
    state: { sorting, columnFilters, globalFilter },
    onSortingChange: setSorting,
    onColumnFiltersChange: setColumnFilters,
    onGlobalFilterChange: setGlobalFilter,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
  })

  if (isLoading) return <FullPageSpinner />

  if (error) {
    return (
      <EmptyState
        icon="⚠"
        title="Failed to load jobs"
        description="Could not connect to the API server. Is agentctl web running?"
      />
    )
  }

  const runningCount = jobs.filter((j) => j.status === 'running').length

  return (
    <div className="flex flex-col h-full">
      <PageHeader
        title="Jobs"
        subtitle={runningCount > 0 ? `${runningCount} running` : `${jobs.length} total`}
        actions={
          <Button variant="ghost" size="sm" onClick={() => refetch()} disabled={isFetching}>
            <RefreshCw size={13} className={cn(isFetching && 'animate-spin')} />
            Refresh
          </Button>
        }
      />

      <div className="flex items-center gap-3 px-6 py-3 border-b border-base-300 bg-base-100/30">
        <div className="relative flex-1 max-w-xs">
          <Search size={13} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-text-dim" />
          <input
            value={globalFilter}
            onChange={(e) => setGlobalFilter(e.target.value)}
            placeholder="Search prompts, agents…"
            className={cn(
              'w-full bg-base-200 border border-base-300 rounded pl-7 pr-8 py-1.5',
              'text-sm text-text placeholder:text-text-dim',
              'focus:outline-none focus:border-accent/50'
            )}
          />
          {globalFilter && (
            <button
              onClick={() => setGlobalFilter('')}
              className="absolute right-2 top-1/2 -translate-y-1/2 text-text-dim hover:text-text"
            >
              <X size={12} />
            </button>
          )}
        </div>

        <select
          value={statusFilter}
          onChange={(e) => setStatusFilter(e.target.value as RunStatus | 'all')}
          className={cn(
            'bg-base-200 border border-base-300 rounded px-3 py-1.5',
            'text-sm text-text focus:outline-none focus:border-accent/50'
          )}
        >
          {STATUS_OPTIONS.map((o) => (
            <option key={o.value} value={o.value}>{o.label}</option>
          ))}
        </select>

        <span className="text-xs text-text-dim ml-auto">
          {table.getFilteredRowModel().rows.length} of {jobs.length}
        </span>
      </div>

      <div className="flex-1 overflow-auto">
        {table.getFilteredRowModel().rows.length === 0 ? (
          <EmptyState
            icon="◦"
            title="No jobs found"
            description={
              jobs.length === 0
                ? 'Start a run from the CLI: agentctl run "your prompt"'
                : 'No jobs match your filters.'
            }
          />
        ) : (
          <table className="w-full text-sm">
            <thead className="sticky top-0 bg-base-100 border-b border-base-300 z-10">
              {table.getHeaderGroups().map((hg) => (
                <tr key={hg.id}>
                  {hg.headers.map((header) => (
                    <th
                      key={header.id}
                      className={cn(
                        'px-4 py-2.5 text-left text-xs font-medium text-text-dim',
                        'whitespace-nowrap select-none',
                        header.column.getCanSort() && 'cursor-pointer hover:text-text'
                      )}
                      onClick={header.column.getToggleSortingHandler()}
                    >
                      <div className="flex items-center gap-1">
                        {flexRender(header.column.columnDef.header, header.getContext())}
                        {header.column.getCanSort() && (
                          <span className="text-text-dim">
                            {header.column.getIsSorted() === 'asc' ? (
                              <ArrowUp size={11} />
                            ) : header.column.getIsSorted() === 'desc' ? (
                              <ArrowDown size={11} />
                            ) : (
                              <ArrowUpDown size={11} className="opacity-40" />
                            )}
                          </span>
                        )}
                      </div>
                    </th>
                  ))}
                </tr>
              ))}
            </thead>
            <tbody className="divide-y divide-base-300/50">
              {table.getRowModel().rows.map((row) => {
                const job = row.original
                const dest = job.run_count > 1
                  ? `/jobs/${job.id}`
                  : `/runs/${job.id}`
                return (
                  <tr
                    key={row.id}
                    onClick={() => navigate(dest)}
                    className={cn(
                      'cursor-pointer transition-colors hover:bg-base-300/30',
                      job.status === 'running' && 'bg-accent/5'
                    )}
                  >
                    {row.getVisibleCells().map((cell) => (
                      <td key={cell.id} className="px-4 py-2.5">
                        {flexRender(cell.column.columnDef.cell, cell.getContext())}
                      </td>
                    ))}
                  </tr>
                )
              })}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}
