import type {
  ListRunsResponse,
  ListEventsResponse,
  ListJobsResponse,
  JobDetail,
  Run,
  CreateRunRequest,
  AgentEvent,
} from './types'

const BASE = '/api/v1'

class ApiError extends Error {
  constructor(
    public readonly status: number,
    message: string
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

async function request<T>(
  path: string,
  options?: RequestInit
): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json', ...options?.headers },
    ...options,
  })

  if (!res.ok) {
    let msg = `HTTP ${res.status}`
    try {
      const body = await res.json() as { error?: string }
      if (body.error) msg = body.error
    } catch {
      // ignore
    }
    throw new ApiError(res.status, msg)
  }

  return res.json() as Promise<T>
}

export const api = {
  health: () =>
    request<{ status: string; version: string }>('/health'),

  jobs: {
    list: (limit = 50) =>
      request<ListJobsResponse>(`/jobs?limit=${limit}`),

    get: (id: string) =>
      request<JobDetail>(`/jobs/${id}`),
  },

  runs: {
    list: (limit = 50) =>
      request<ListRunsResponse>(`/runs?limit=${limit}`),

    get: (id: string) =>
      request<Run>(`/runs/${id}`),

    create: (body: CreateRunRequest) =>
      request<Run>('/runs', {
        method: 'POST',
        body: JSON.stringify(body),
      }),

    cancel: (id: string) =>
      request<{ status: string }>(`/runs/${id}/cancel`, { method: 'POST' }),

    listChildren: (id: string) =>
      request<ListRunsResponse>(`/runs/${id}/children`),

    getTree: (id: string) =>
      request<ListRunsResponse>(`/runs/${id}/tree`),
  },

  events: {
    list: (runId: string) =>
      request<ListEventsResponse>(`/runs/${runId}/events`),

    streamUrl: (runId: string) =>
      `${BASE}/runs/${runId}/stream`,
  },
}

// Re-export for convenience
export { ApiError }

// SSE helper — returns a cleanup function
export function subscribeToRunStream(
  runId: string,
  handlers: {
    onEvent?: (event: AgentEvent) => void
    onRunUpdate?: (run: Run) => void
    onDone?: () => void
    onError?: (msg: string) => void
  }
): () => void {
  const es = new EventSource(api.events.streamUrl(runId))

  es.addEventListener('event', (e) => {
    try {
      const event = JSON.parse(e.data) as AgentEvent
      handlers.onEvent?.(event)
    } catch {
      // ignore parse errors
    }
  })

  es.addEventListener('run', (e) => {
    try {
      const run = JSON.parse(e.data) as Run
      handlers.onRunUpdate?.(run)
    } catch {
      // ignore
    }
  })

  es.addEventListener('done', () => {
    es.close()
    handlers.onDone?.()
  })

  es.addEventListener('error', (e) => {
    if (e instanceof MessageEvent) {
      try {
        const data = JSON.parse(e.data) as { message?: string }
        handlers.onError?.(data.message ?? 'stream error')
      } catch {
        handlers.onError?.('stream error')
      }
    }
    es.close()
  })

  // onerror fires on connection failures
  es.onerror = () => {
    handlers.onError?.('connection lost')
    es.close()
  }

  return () => es.close()
}
