import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState, useCallback } from 'react'
import { api, subscribeToRunStream } from '@/api/client'
import type { Run, AgentEvent } from '@/api/types'

// ── Query keys ────────────────────────────────────────────────────────────────

export const queryKeys = {
  jobs: (limit?: number) => ['jobs', limit] as const,
  job: (id: string) => ['jobs', id] as const,
  runs: (limit?: number) => ['runs', limit] as const,
  run: (id: string) => ['runs', id] as const,
  childRuns: (id: string) => ['runs', id, 'children'] as const,
  events: (runId: string) => ['events', runId] as const,
}

// ── Job queries ───────────────────────────────────────────────────────────────

export function useJobs(limit = 50) {
  return useQuery({
    queryKey: queryKeys.jobs(limit),
    queryFn: () => api.jobs.list(limit),
    refetchInterval: 3000,
    select: (data) => data.jobs,
  })
}

export function useJob(id: string) {
  return useQuery({
    queryKey: queryKeys.job(id),
    queryFn: () => api.jobs.get(id),
    enabled: !!id,
    refetchInterval: 5000,
  })
}

// ── Run queries ───────────────────────────────────────────────────────────────

export function useRuns(limit = 50) {
  return useQuery({
    queryKey: queryKeys.runs(limit),
    queryFn: () => api.runs.list(limit),
    refetchInterval: 3000, // Poll for running-status changes
    select: (data) => data.runs,
  })
}

export function useRun(id: string) {
  return useQuery({
    queryKey: queryKeys.run(id),
    queryFn: () => api.runs.get(id),
    enabled: !!id,
  })
}

export function useEvents(runId: string) {
  return useQuery({
    queryKey: queryKeys.events(runId),
    queryFn: () => api.events.list(runId),
    enabled: !!runId,
    select: (data) => data.events,
  })
}

export function useChildRuns(runId: string) {
  return useQuery({
    queryKey: queryKeys.childRuns(runId),
    queryFn: () => api.runs.listChildren(runId),
    enabled: !!runId,
    refetchInterval: 5000,
    select: (data) => data.runs,
  })
}

export function useRunTree(runId: string) {
  return useQuery({
    queryKey: ['runs', runId, 'tree'] as const,
    queryFn: () => api.runs.getTree(runId),
    enabled: !!runId,
    refetchInterval: 5000,
    select: (data) => data.runs,
  })
}

// ── Mutations ─────────────────────────────────────────────────────────────────

export function useCreateRun() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: api.runs.create,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['runs'] })
    },
  })
}

export function useCancelRun() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: api.runs.cancel,
    onSuccess: (_data, runId) => {
      qc.invalidateQueries({ queryKey: queryKeys.run(runId) })
      qc.invalidateQueries({ queryKey: ['runs'] })
    },
  })
}

// ── Live SSE stream ───────────────────────────────────────────────────────────

export interface LiveRunState {
  run: Run | null
  events: AgentEvent[]
  isDone: boolean
  error: string | null
}

export function useLiveRun(runId: string, initialRun: Run | null): LiveRunState {
  const qc = useQueryClient()
  const [state, setState] = useState<LiveRunState>({
    run: initialRun,
    events: [],
    isDone: false,
    error: null,
  })

  // Stable callback refs to avoid resubscribing on every render
  const runIdRef = useRef(runId)
  runIdRef.current = runId

  const invalidate = useCallback(() => {
    qc.invalidateQueries({ queryKey: queryKeys.run(runIdRef.current) })
    qc.invalidateQueries({ queryKey: ['runs'] })
  }, [qc])

  useEffect(() => {
    if (!runId) return

    setState({ run: initialRun, events: [], isDone: false, error: null })

    const cleanup = subscribeToRunStream(runId, {
      onEvent: (event) =>
        setState((prev) => ({
          ...prev,
          events: prev.events.some((e) => e.seq === event.seq)
            ? prev.events
            : [...prev.events, event].sort((a, b) => a.seq - b.seq),
        })),
      onRunUpdate: (run) =>
        setState((prev) => ({ ...prev, run })),
      onDone: () => {
        setState((prev) => ({ ...prev, isDone: true }))
        invalidate()
      },
      onError: (msg) =>
        setState((prev) => ({ ...prev, error: msg, isDone: true })),
    })

    return cleanup
  }, [runId]) // eslint-disable-line react-hooks/exhaustive-deps

  return state
}
