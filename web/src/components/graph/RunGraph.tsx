import { useMemo, useEffect } from 'react'
import {
  ReactFlow,
  ReactFlowProvider,
  Background,
  Controls,
  Handle,
  Position,
  MarkerType,
  useNodesState,
  useEdgesState,
  useReactFlow,
  type Node,
  type Edge,
  type NodeTypes,
  BackgroundVariant,
} from '@xyflow/react'
import { useNavigate } from 'react-router-dom'
import dagre from 'dagre'
import '@xyflow/react/dist/style.css'
import { cn, shortId, formatDuration, formatCost } from '@/lib/utils'
import type { Run } from '@/api/types'

// ── Node data ─────────────────────────────────────────────────────────────────

interface RunNodeData extends Record<string, unknown> {
  run: Run
}

// ── Layout ────────────────────────────────────────────────────────────────────

const NODE_W = 220
const NODE_H = 80

function buildGraph(runs: Run[]): { nodes: Node[]; edges: Edge[] } {
  const g = new dagre.graphlib.Graph()
  g.setDefaultEdgeLabel(() => ({}))
  g.setGraph({ rankdir: 'LR', ranksep: 80, nodesep: 40 })

  for (const run of runs) {
    g.setNode(run.id, { width: NODE_W, height: NODE_H })
  }

  const edges: Edge[] = []
  for (const run of runs) {
    if (!run.parent_run_id) continue
    g.setEdge(run.parent_run_id, run.id)
    edges.push({
      id: `${run.parent_run_id}->${run.id}`,
      source: run.parent_run_id,
      target: run.id,
      markerEnd: { type: MarkerType.ArrowClosed, width: 12, height: 12, color: '#4b5563' },
      style: { stroke: '#4b5563', strokeWidth: 1.5 },
    })
  }

  dagre.layout(g)

  const nodes: Node[] = runs.map((run) => {
    const pos = g.node(run.id)
    return {
      id: run.id,
      type: 'run',
      position: { x: pos.x - NODE_W / 2, y: pos.y - NODE_H / 2 },
      data: { run } satisfies RunNodeData,
    }
  })

  return { nodes, edges }
}

// ── RunNode component ─────────────────────────────────────────────────────────

const STATUS_COLORS: Record<string, string> = {
  completed: 'bg-green/20 border-green/40 text-green',
  failed:    'bg-red/20 border-red/40 text-red',
  running:   'bg-accent/20 border-accent/40 text-accent',
  pending:   'bg-yellow/20 border-yellow/40 text-yellow',
  cancelled: 'bg-base-300 border-base-400 text-text-dim',
}

function RunNode({ data }: { data: RunNodeData }) {
  const navigate = useNavigate()
  const { run } = data
  const colorClass = STATUS_COLORS[run.status] ?? STATUS_COLORS.cancelled
  const prompt = run.prompt.length > 48 ? run.prompt.slice(0, 46) + '…' : run.prompt

  return (
    <>
      <Handle type="target" position={Position.Left} className="!border-0 !bg-base-400 !w-2 !h-2" />
      <div
        onClick={() => navigate(`/runs/${run.id}`)}
        className={cn(
          'w-[220px] rounded border px-3 py-2 cursor-pointer select-none',
          'bg-base-200 border-base-300 hover:border-base-400 transition-colors',
        )}
        style={{ minHeight: NODE_H }}
      >
        {/* Header row */}
        <div className="flex items-center gap-1.5 mb-1">
          <span className={cn('inline-block w-1.5 h-1.5 rounded-full shrink-0', {
            'bg-green':    run.status === 'completed',
            'bg-red':      run.status === 'failed',
            'bg-accent animate-pulse': run.status === 'running',
            'bg-yellow':   run.status === 'pending',
            'bg-base-400': run.status === 'cancelled',
          })} />
          <span className="font-mono text-accent text-[10px] font-semibold shrink-0">
            {shortId(run.id)}
          </span>
          {run.agent_id && (
            <span className="text-[10px] text-text-dim truncate font-mono">
              {run.agent_id}
            </span>
          )}
        </div>

        {/* Prompt */}
        <p className="text-xs text-text leading-snug mb-2">{prompt}</p>

        {/* Footer stats */}
        <div className="flex items-center gap-2 text-[10px] text-text-dim font-mono">
          <span className={cn('px-1 rounded text-[9px]', colorClass)}>
            {run.status}
          </span>
          <span>{formatDuration(run.duration_ms)}</span>
          {run.cost_usd > 0 && <span>{formatCost(run.cost_usd)}</span>}
        </div>
      </div>
      <Handle type="source" position={Position.Right} className="!border-0 !bg-base-400 !w-2 !h-2" />
    </>
  )
}

const nodeTypes: NodeTypes = { run: RunNode }

// ── Inner component (needs ReactFlowProvider context) ─────────────────────────

function RunGraphInner({ runs, className }: { runs: Run[]; className?: string }) {
  const { fitView } = useReactFlow()
  const [nodes, setNodes, onNodesChange] = useNodesState<Node>([])
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([])

  const graphData = useMemo(() => buildGraph(runs), [runs])

  useEffect(() => {
    setNodes(graphData.nodes)
    setEdges(graphData.edges)
    requestAnimationFrame(() => fitView({ padding: 0.2, duration: 200 }))
  }, [graphData, setNodes, setEdges, fitView])

  return (
    <div className={cn('bg-base-100', className)}>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        nodeTypes={nodeTypes}
        proOptions={{ hideAttribution: true }}
        nodesDraggable
        nodesConnectable={false}
        elementsSelectable={false}
      >
        <Background variant={BackgroundVariant.Dots} gap={16} size={1} className="opacity-30" />
        <Controls showInteractive={false} />
      </ReactFlow>
    </div>
  )
}

// ── Public export ─────────────────────────────────────────────────────────────

export function RunGraph({ runs, className }: { runs: Run[]; className?: string }) {
  return (
    <ReactFlowProvider>
      <RunGraphInner runs={runs} className={className} />
    </ReactFlowProvider>
  )
}
