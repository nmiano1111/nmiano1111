import { useMemo, useEffect } from 'react'
import {
  ReactFlow,
  ReactFlowProvider,
  Background,
  Controls,
  MiniMap,
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
import dagre from 'dagre'
import '@xyflow/react/dist/style.css'
import { cn } from '@/lib/utils'
import type { AgentEvent } from '@/api/types'

// ── High-level step model ─────────────────────────────────────────────────────

type StepKind = 'reasoning' | 'tool_cluster' | 'completed' | 'failed' | 'verification'

interface ToolEntry {
  name: string
  count: number
  errorCount: number
}

interface Step {
  id: string
  kind: StepKind
  label: string          // primary text
  sublabel?: string      // secondary / command text
  // tool_cluster only
  tools?: ToolEntry[]    // per-tool breakdown
  totalCalls?: number
  totalErrors?: number
  // reasoning only
  textCount?: number
  // terminal / verify
  isError?: boolean
}


function computeSteps(events: AgentEvent[]): Step[] {
  // Map tool_use seq → its tool_result event
  const resultByUseSeq = new Map<number, AgentEvent>()
  for (const e of events) {
    if (e.kind === 'tool_result' && e.parent_seq != null) {
      resultByUseSeq.set(e.parent_seq, e)
    }
  }
  const absorbedSeqs = new Set<number>()
  for (const e of resultByUseSeq.values()) absorbedSeqs.add(e.seq)

  const steps: Step[] = []
  let pendingText: AgentEvent[] = []
  // Accumulate consecutive tool_use events into a cluster
  let pendingTools: Array<{ name: string; isError: boolean }> = []
  let idx = 0

  const flushText = () => {
    if (pendingText.length === 0) return
    const first = (pendingText[0].data ?? {}) as Record<string, unknown>
    const content = String(first.content ?? '').replace(/\n/g, ' ').trim()
    const label =
      pendingText.length > 1
        ? `Reasoning · ${pendingText.length} messages`
        : content.slice(0, 70) || 'Reasoning'
    steps.push({ id: `reasoning-${idx++}`, kind: 'reasoning', label, textCount: pendingText.length })
    pendingText = []
  }

  const flushTools = () => {
    if (pendingTools.length === 0) return
    // Count per tool name, preserving first-seen order
    const byName = new Map<string, ToolEntry>()
    for (const t of pendingTools) {
      const entry = byName.get(t.name) ?? { name: t.name, count: 0, errorCount: 0 }
      entry.count++
      if (t.isError) entry.errorCount++
      byName.set(t.name, entry)
    }
    const tools = Array.from(byName.values())
    const totalErrors = pendingTools.filter((t) => t.isError).length
    const n = pendingTools.length
    steps.push({
      id: `tools-${idx++}`,
      kind: 'tool_cluster',
      label: `${n} tool call${n > 1 ? 's' : ''}`,
      tools,
      totalCalls: n,
      totalErrors,
    })
    pendingTools = []
  }

  for (const e of events) {
    if (absorbedSeqs.has(e.seq)) continue
    if (e.kind === 'started') continue

    if (e.kind === 'text') {
      flushTools()
      pendingText.push(e)
      continue
    }

    if (e.kind === 'tool_use') {
      flushText()
      const rd = resultByUseSeq.get(e.seq)
      const rdData = rd ? (rd.data ?? {}) as Record<string, unknown> : null
      const d = (e.data ?? {}) as Record<string, unknown>
      pendingTools.push({
        name: String(d.name ?? 'tool'),
        isError: rdData ? Boolean(rdData.is_error) : false,
      })
      continue
    }

    // Non-tool, non-text event — flush both accumulators first
    flushTools()
    flushText()

    const d = (e.data ?? {}) as Record<string, unknown>

    if (e.kind === 'completed') {
      steps.push({
        id: `completed-${e.seq}`,
        kind: 'completed',
        label: String(d.result ?? '').slice(0, 100).replace(/\n/g, ' ').trim() || 'Done',
      })
    } else if (e.kind === 'failed') {
      steps.push({
        id: `failed-${e.seq}`,
        kind: 'failed',
        label: String(d.error ?? 'Failed').slice(0, 100).replace(/\n/g, ' ').trim(),
        isError: true,
      })
    } else if (e.kind === 'verification') {
      const passed = Boolean(d.passed)
      steps.push({
        id: `verification-${e.seq}`,
        kind: 'verification',
        label: passed ? 'Verified' : 'Verify failed',
        sublabel: String(d.command ?? '').slice(0, 60),
        isError: !passed,
      })
    }
    idx++
  }

  flushTools()
  flushText()
  return steps
}

// ── Node component ────────────────────────────────────────────────────────────

interface StepNodeData extends Record<string, unknown> {
  step: Step
}

function StepNode({ data }: { data: StepNodeData }) {
  const { step } = data
  const handle = (type: 'source' | 'target', pos: Position) => (
    <Handle type={type} position={pos}
      style={{ background: '#585b70', width: 6, height: 6, border: 'none' }} />
  )

  if (step.kind === 'tool_cluster') {
    const hasErrors = (step.totalErrors ?? 0) > 0
    return (
      <div className="rounded border bg-mauve/5 border-mauve/30 min-w-[180px] max-w-[260px] select-none">
        {handle('target', Position.Left)}
        <div className="px-3 pt-2 pb-2">
          <div className="text-[10px] font-semibold uppercase tracking-wider text-mauve/60 mb-1.5">
            ⚡ {step.totalCalls} Tool Call{step.totalCalls !== 1 ? 's' : ''}
          </div>
          <div className="flex flex-wrap gap-x-3 gap-y-0.5">
            {step.tools?.map((t) => (
              <span key={t.name}
                className={cn('text-[11px]', t.errorCount > 0 ? 'text-red/80' : 'text-text-dim')}
              >
                {t.name}{t.count > 1 ? <span className="text-text-dim/60"> ×{t.count}</span> : null}
              </span>
            ))}
          </div>
          {hasErrors && (
            <div className="text-[10px] text-red/60 mt-1.5">
              {step.totalErrors} error{step.totalErrors !== 1 ? 's' : ''}
            </div>
          )}
        </div>
        {handle('source', Position.Right)}
      </div>
    )
  }

  if (step.kind === 'reasoning') {
    return (
      <div className="rounded border bg-base-100/50 border-base-300/50 min-w-[160px] max-w-[240px] select-none">
        {handle('target', Position.Left)}
        <div className="px-3 py-2">
          <div className="text-[10px] font-semibold uppercase tracking-wider text-text-dim/50 mb-1">
            💭 Reasoning
          </div>
          <div className="text-[11px] text-text-dim leading-snug" style={{ maxWidth: 210 }}>
            {step.label}
          </div>
        </div>
        {handle('source', Position.Right)}
      </div>
    )
  }

  // completed / failed / verification
  const terminalStyle: Record<string, { border: string; bg: string; icon: string; color: string }> = {
    completed:    { border: 'border-green/40',  bg: 'bg-green/5',  icon: '✓', color: 'text-green' },
    failed:       { border: 'border-red/40',    bg: 'bg-red/5',    icon: '✗', color: 'text-red' },
    verification: {
      border: step.isError ? 'border-red/30' : 'border-sky/30',
      bg:     step.isError ? 'bg-red/5'      : 'bg-sky/5',
      icon:   step.isError ? '✗'             : '✓',
      color:  step.isError ? 'text-red'      : 'text-sky',
    },
  }
  const ts = terminalStyle[step.kind] ?? terminalStyle.completed

  return (
    <div className={cn('rounded border min-w-[160px] max-w-[240px] select-none', ts.border, ts.bg)}>
      {handle('target', Position.Left)}
      <div className="px-3 py-2">
        <div className={cn('text-[10px] font-semibold uppercase tracking-wider mb-1 opacity-70', ts.color)}>
          {ts.icon} {step.kind === 'verification' ? 'Verify' : step.kind === 'completed' ? 'Done' : 'Failed'}
        </div>
        <div className={cn('text-[11px] leading-snug', ts.color)} style={{ maxWidth: 210 }}>
          {step.label}
        </div>
        {step.sublabel && (
          <div className="text-[10px] text-text-dim font-mono mt-0.5 truncate" style={{ maxWidth: 210 }}>
            {step.sublabel}
          </div>
        )}
      </div>
      {handle('source', Position.Right)}
    </div>
  )
}

const nodeTypes: NodeTypes = { step: StepNode }

// ── Graph builder ─────────────────────────────────────────────────────────────

const NODE_W = 260
// Heights vary by kind — tell dagre the real rendered size so it doesn't overlap nodes
function nodeHeight(step: Step): number {
  if (step.kind === 'tool_cluster') {
    const rows = Math.ceil((step.tools?.length ?? 0) / 3) // ~3 tool names per row
    return 52 + rows * 16 + (step.totalErrors ? 16 : 0)
  }
  return 58
}

function buildGraph(events: AgentEvent[]): { nodes: Node[]; edges: Edge[] } {
  const steps = computeSteps(events)
  if (steps.length === 0) return { nodes: [], edges: [] }

  const nodes: Node[] = steps.map((step): Node => ({
    id: step.id,
    type: 'step',
    position: { x: 0, y: 0 },
    data: { step } satisfies StepNodeData,
  }))

  const edges: Edge[] = []
  for (let i = 1; i < steps.length; i++) {
    const prev = steps[i - 1]
    const curr = steps[i]
    const isDim = prev.kind === 'reasoning' || curr.kind === 'reasoning'
    edges.push({
      id: `e-${i}`,
      source: prev.id,
      target: curr.id,
      style: {
        stroke: isDim ? '#45475a' : '#585b70',
        strokeWidth: 1.5,
        strokeDasharray: isDim ? '4 3' : undefined,
      },
      markerEnd: { type: MarkerType.ArrowClosed, color: '#585b70', width: 13, height: 13 },
    })
  }

  const g = new dagre.graphlib.Graph()
  g.setDefaultEdgeLabel(() => ({}))
  g.setGraph({ rankdir: 'LR', ranksep: 80, nodesep: 40 })

  nodes.forEach((node) => {
    const step = (node.data as StepNodeData).step
    g.setNode(node.id, { width: NODE_W, height: nodeHeight(step) })
  })
  edges.forEach((e) => g.setEdge(e.source, e.target))
  dagre.layout(g)

  const positioned = nodes.map((node) => {
    const { x, y } = g.node(node.id)
    const step = (node.data as StepNodeData).step
    const h = nodeHeight(step)
    return { ...node, position: { x: x - NODE_W / 2, y: y - h / 2 } }
  })

  return { nodes: positioned, edges }
}

// ── Component ─────────────────────────────────────────────────────────────────

interface EventGraphProps {
  events: AgentEvent[]
  className?: string
}

function EventGraphInner({ events, className }: EventGraphProps) {
  const { fitView } = useReactFlow()
  const [nodes, setNodes, onNodesChange] = useNodesState<Node>([])
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([])

  const graphData = useMemo(() => buildGraph(events), [events])

  useEffect(() => {
    setNodes(graphData.nodes)
    setEdges(graphData.edges)
    // fitView after React has committed the new nodes to the DOM.
    requestAnimationFrame(() => fitView({ padding: 0.25, duration: 200 }))
  }, [graphData, setNodes, setEdges, fitView])

  if (events.length === 0) {
    return (
      <div className={cn('flex items-center justify-center text-sm text-text-dim', className)}>
        No events to visualize
      </div>
    )
  }

  return (
    <div className={cn('relative', className)}>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        nodeTypes={nodeTypes}
        minZoom={0.2}
        maxZoom={2}
        proOptions={{ hideAttribution: true }}
      >
        <Background color="#313244" gap={20} variant={BackgroundVariant.Dots} size={1} />
        <Controls style={{ background: '#1e1e2e', border: '1px solid #313244', borderRadius: 6 }} />
        <MiniMap
          style={{ background: '#181825', border: '1px solid #313244' }}
          nodeColor={(node) => {
            const kind = ((node.data as StepNodeData).step ?? {}).kind
            if (kind === 'completed')    return '#a6e3a1'
            if (kind === 'failed')       return '#f38ba8'
            if (kind === 'tool_cluster') return '#cba6f7'
            if (kind === 'verification') return '#89dceb'
            return '#45475a'
          }}
        />
      </ReactFlow>
    </div>
  )
}

export function EventGraph(props: EventGraphProps) {
  return (
    <ReactFlowProvider>
      <EventGraphInner {...props} />
    </ReactFlowProvider>
  )
}
