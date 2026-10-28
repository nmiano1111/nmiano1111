// Domain types mirroring Go structs in internal/agent/types.go
// Timestamps are RFC3339 strings from the Go JSON encoder.

export type RunStatus = 'pending' | 'running' | 'completed' | 'failed' | 'cancelled'

export interface Run {
  id: string
  agent_id: string
  prompt: string
  status: RunStatus
  created_at: string
  started_at: string | null
  completed_at: string | null
  result: string
  error: string
  cost_usd: number
  input_tokens: number
  output_tokens: number
  duration_ms: number
  runner_name: string
  model: string
  parent_run_id: string
  pid: number
}

export type EventKind =
  | 'started'
  | 'text'
  | 'tool_use'
  | 'tool_result'
  | 'completed'
  | 'failed'
  | 'verification'

export interface AgentEvent {
  id: string
  run_id: string
  kind: EventKind
  timestamp: string
  seq: number
  parent_seq: number | null
  data: EventData
}

// Typed event payloads
export interface StartedData {
  pid: number
}

export interface TextData {
  content: string
}

export interface ToolUseData {
  tool_id: string
  name: string
  input: unknown
}

export interface ToolResultData {
  tool_id: string
  content: string
  is_error: boolean
}

export interface CompletedData {
  result: string
  cost_usd: number
  input_tokens: number
  output_tokens: number
  duration_ms: number
}

export interface FailedData {
  error: string
}

export interface VerificationData {
  command: string
  passed: boolean
  output: string
}

export type EventData =
  | StartedData
  | TextData
  | ToolUseData
  | ToolResultData
  | CompletedData
  | FailedData
  | VerificationData
  | Record<string, unknown>

// API response shapes
export interface ListRunsResponse {
  runs: Run[]
}

export interface ListEventsResponse {
  events: AgentEvent[]
}

export interface CreateRunRequest {
  prompt: string
  agent_id?: string
  model?: string
  workdir?: string
  runner?: 'claude' | 'claude-code' | 'fake'
  timeout_ms?: number
  verify?: string
  isolate?: boolean
  parent_run_id?: string
}

export interface ApiError {
  error: string
}
