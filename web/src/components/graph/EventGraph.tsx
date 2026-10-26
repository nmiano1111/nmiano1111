import { useMemo } from 'react'
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  useNodesState,
  useEdgesState,
  type Node,
  type Edge,
  type NodeTypes,
  BackgroundVariant,
} from '@xyflow/react'
import dagre from 'dagre'
import '@xyflow/react/dist/style.css'
import { cn, EVENT_CONFIG } from '@/lib/utils'
import type { AgentEvent } from '@/api/types'

// Custom node component for events
function EventNode({ data }: { data: { label: string; kind: string; detail: string; isTerminal: boolean } }) {
  const cfg = EVENT_CONFIG[data.kind as keyof typeof EVENT_CONFIG] ?? {
    label: data.kind,
    icon: '·',
    color: 'text-text-dim',
  }

  return (
    <div
      className={cn(
        'px-3 py-2 rounded border text-xs min-w-[130px] max-w-[200px]',
        'bg-base-200 border-base-300',
        data.kind === 'completed' && 'border-green/40 bg-green/5',
        data.kind === 'failed' && 'border-red/40 bg-red/5',
        data.kind === 'tool_use' && 'border-mauve/40 bg-mauve/5',
        data.kind === 'tool_result' && 'border-peach/40 bg-peach/5',
        data.kind === 'started' && 'border-accent/40 bg-accent/5',
        data.kind === 'verification' && 'border-sky/40 bg-sky/5',
      )}
    >
      <div className={cn('font-medium mb-0.5', cfg.color)}>
        {cfg.icon} {cfg.label}
      </div>
      {data.detail && (
        <div className="text-text-dim truncate" style={{ maxWidth: 160 }}>
          {data.detail}
        </div>
      )}
    </div>
  )
}

const nodeTypes: NodeTypes = {
  event: EventNode,
}

function getEventDetail(event: AgentEvent): string {
  const d = (event.data ?? {}) as Record<string, unknown>
  switch (event.kind) {
    case 'text': {
      const content = String(d.content ?? '').replace(/\n/g, ' ').trim()
      return content.length > 40 ? content.slice(0, 38) + '…' : content
    }
    case 'tool_use':
      return String(d.name ?? '')
    case 'tool_result':
      return d.is_error ? '[error]' : String(d.content ?? '').slice(0, 40).replace(/\n/g, ' ').trim()
    case 'completed':
      return String(d.result ?? '').slice(0, 40) || 'done'
    case 'failed':
      return String(d.error ?? '').slice(0, 40)
    case 'verification':
      return (d.passed ? '✓ ' : '✗ ') + String(d.command ?? '')
    case 'started':
      return `pid ${d.pid ?? '?'}`
    default:
      return ''
  }
}

function buildGraph(events: AgentEvent[]): { nodes: Node[]; edges: Edge[] } {
  if (events.length === 0) return { nodes: [], edges: [] }

  // Build nodes
  const nodes: Node[] = events.map((event) => ({
    id: String(event.seq),
    type: 'event',
    position: { x: 0, y: 0 }, // Dagre will set positions
    data: {
      label: event.kind,
      kind: event.kind,
      detail: getEventDetail(event),
      isTerminal: event.kind === 'completed' || event.kind === 'failed',
    },
  }))

  // Build edges: sequential + parent_seq links
  const edges: Edge[] = []
  for (let i = 1; i < events.length; i++) {
    const prev = events[i - 1]
    const curr = events[i]
    // Sequential edge
    edges.push({
      id: `seq-${prev.seq}-${curr.seq}`,
      source: String(prev.seq),
      target: String(curr.seq),
      style: { stroke: '#45475a', strokeWidth: 1.5 },
      animated: false,
    })
    // Parent link (tool_use → tool_result)
    if (curr.parent_seq !== null && curr.parent_seq !== undefined) {
      edges.push({
        id: `parent-${curr.parent_seq}-${curr.seq}`,
        source: String(curr.parent_seq),
        target: String(curr.seq),
        style: { stroke: '#cba6f7', strokeWidth: 1, strokeDasharray: '4 2' },
        label: 'result',
        labelStyle: { fill: '#6c7086', fontSize: 9 },
      })
    }
  }

  // Auto-layout with Dagre
  const g = new dagre.graphlib.Graph()
  g.setDefaultEdgeLabel(() => ({}))
  g.setGraph({ rankdir: 'LR', ranksep: 60, nodesep: 20 })

  nodes.forEach((node) => {
    g.setNode(node.id, { width: 200, height: 52 })
  })
  edges.forEach((edge) => {
    g.setEdge(edge.source, edge.target)
  })

  dagre.layout(g)

  const positioned = nodes.map((node) => {
    const { x, y } = g.node(node.id)
    return { ...node, position: { x: x - 100, y: y - 26 } }
  })

  return { nodes: positioned, edges }
}

interface EventGraphProps {
  events: AgentEvent[]
  className?: string
}

export function EventGraph({ events, className }: EventGraphProps) {
  const { nodes: initialNodes, edges: initialEdges } = useMemo(
    () => buildGraph(events),
    [events]
  )

  const [nodes, , onNodesChange] = useNodesState(initialNodes)
  const [edges, , onEdgesChange] = useEdgesState(initialEdges)

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
        fitView
        fitViewOptions={{ padding: 0.2 }}
        minZoom={0.3}
        maxZoom={2}
        proOptions={{ hideAttribution: true }}
      >
        <Background
          color="#313244"
          gap={20}
          variant={BackgroundVariant.Dots}
          size={1}
        />
        <Controls
          style={{
            background: '#1e1e2e',
            border: '1px solid #313244',
            borderRadius: 6,
          }}
        />
        <MiniMap
          style={{
            background: '#181825',
            border: '1px solid #313244',
          }}
          nodeColor={(node) => {
            const kind = (node.data as { kind: string }).kind
            if (kind === 'completed') return '#a6e3a1'
            if (kind === 'failed') return '#f38ba8'
            if (kind === 'tool_use') return '#cba6f7'
            if (kind === 'tool_result') return '#fab387'
            return '#45475a'
          }}
        />
      </ReactFlow>
    </div>
  )
}
