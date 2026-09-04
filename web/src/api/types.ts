import { useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from './client'

export interface Endpoint {
  id: number
  name: string
  url: string
  type: string // "http" | "https" | "tcp" | "dns" | "ping"
  interval_seconds: number
  status: string // "unknown" | "ok" | "not_ok"
  paused: boolean
  last_status_code: number
  last_latency_ms: number
  last_check_error: string
  last_checked_at: string | null
  last_failure_at: string | null
  consecutive_failures: number
  expected_status: number // 0 = any 2xx
  expected_keyword: string
  cert_expires_at: string | null
  paused_until: string | null
  created_at: string
}

export interface Stats {
  total: number
  up: number
  uptime: number
  avg_latency_ms: number
  p95_latency_ms: number
  incidents: number
}

export interface Incident {
  start: string
  duration_seconds: number // 0 = ongoing
  status_code: number
}

export interface Check {
  checked_at: string
  up: boolean
  status_code: number
  latency_ms: number
}

export interface DayStat {
  date: string // YYYY-MM-DD (UTC)
  total: number
  up: number
  uptime: number
  avg_latency_ms: number
}

export type StatsWindow = '24h' | '7d' | '30d'

export interface CreateEndpointInput {
  name: string
  url: string
  interval_seconds?: number
}

export interface UpdateEndpointInput {
  name?: string
  interval_seconds?: number
  expected_status?: number
  expected_keyword?: string
}

export interface MaintenanceWindow {
  id: number
  endpoint_id: number | null // null = applies to ALL endpoints
  days: string // "all" or comma-separated day codes: "mon,wed,fri"
  start_minutes: number // minutes since midnight UTC
  end_minutes: number // end < start = overnight window
  active: boolean // window is in effect right now (UTC)
}

export interface CreateMaintenanceInput {
  endpoint_id?: number | null
  days: string
  start: string // "HH:MM" UTC
  end: string // "HH:MM" UTC
}

export function useMaintenanceWindows() {
  return useQuery({
    queryKey: ['maintenance'],
    queryFn: () =>
      api<{ maintenance: MaintenanceWindow[] }>('/api/maintenance').then((r) => r.maintenance),
    refetchInterval: 60000,
  })
}

export function useAddMaintenanceWindow() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateMaintenanceInput) =>
      api<{ maintenance: MaintenanceWindow }>('/api/maintenance', {
        method: 'POST',
        body: JSON.stringify(input),
      }).then((r) => r.maintenance),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['maintenance'] })
      void queryClient.invalidateQueries({ queryKey: ['endpoints'] })
    },
  })
}

export function useDeleteMaintenanceWindow() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => api<void>(`/api/maintenance/${id}`, { method: 'DELETE' }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['maintenance'] })
      void queryClient.invalidateQueries({ queryKey: ['endpoints'] })
    },
  })
}

export function useEndpoints() {
  return useQuery({
    queryKey: ['endpoints'],
    queryFn: () => api<{ endpoints: Endpoint[] }>('/api/endpoints').then((r) => r.endpoints),
    refetchInterval: 15000,
  })
}

export function useEndpoint(id: number) {
  return useQuery({
    queryKey: ['endpoints', id],
    queryFn: () =>
      api<{ endpoint: Endpoint; stats: Record<StatsWindow, Stats> }>(`/api/endpoints/${id}`),
  })
}

export function useIncidents(id: number) {
  return useQuery({
    queryKey: ['endpoints', id, 'incidents'],
    queryFn: () =>
      api<{ incidents: Incident[] }>(`/api/endpoints/${id}/incidents`).then((r) => r.incidents),
  })
}

export function useChecks(id: number, window: StatsWindow) {
  return useQuery({
    queryKey: ['endpoints', id, 'checks', window],
    queryFn: () =>
      api<{ checks: Check[] }>(`/api/endpoints/${id}/checks?window=${window}`).then(
        (r) => r.checks,
      ),
  })
}

// 24h checks for every endpoint at once (dashboard uptime bars).
// Result order matches the input array; shares the cache with useChecks(id, '24h').
export function useChecks24h(endpoints: Endpoint[] | undefined) {
  return useQueries({
    queries: (endpoints ?? []).map((endpoint) => ({
      queryKey: ['endpoints', endpoint.id, 'checks', '24h' as const],
      queryFn: () =>
        api<{ checks: Check[] }>(`/api/endpoints/${endpoint.id}/checks?window=24h`).then(
          (r) => r.checks,
        ),
      refetchInterval: 60000,
      staleTime: 30000,
    })),
  })
}

const DAILY_DAYS = 30

export function useDailyStats(id: number) {
  return useQuery({
    queryKey: ['endpoints', id, 'daily'],
    queryFn: () =>
      api<{ days: DayStat[] }>(`/api/endpoints/${id}/daily?days=${DAILY_DAYS}`).then(
        (r) => r.days,
      ),
    staleTime: 60000,
  })
}

// Daily stats for every endpoint at once (dashboard 30d uptime column).
// Result order matches the input array; shares the cache with useDailyStats(id).
export function useDailyStatsAll(endpoints: Endpoint[] | undefined) {
  return useQueries({
    queries: (endpoints ?? []).map((endpoint) => ({
      queryKey: ['endpoints', endpoint.id, 'daily'],
      queryFn: () =>
        api<{ days: DayStat[] }>(`/api/endpoints/${endpoint.id}/daily?days=${DAILY_DAYS}`).then(
          (r) => r.days,
        ),
      staleTime: 60000,
    })),
  })
}

function useInvalidateEndpoints() {
  const queryClient = useQueryClient()
  return (id?: number) => {
    void queryClient.invalidateQueries({ queryKey: ['endpoints'] })
    if (id !== undefined) {
      void queryClient.invalidateQueries({ queryKey: ['endpoints', id] })
    }
  }
}

export function useCreateEndpoint() {
  const invalidate = useInvalidateEndpoints()
  return useMutation({
    mutationFn: (input: CreateEndpointInput) =>
      api<{ endpoint: Endpoint }>('/api/endpoints', {
        method: 'POST',
        body: JSON.stringify(input),
      }).then((r) => r.endpoint),
    onSuccess: () => invalidate(),
  })
}

export function useUpdateEndpoint(id: number) {
  const invalidate = useInvalidateEndpoints()
  return useMutation({
    mutationFn: (input: UpdateEndpointInput) =>
      api<{ endpoint: Endpoint }>(`/api/endpoints/${id}`, {
        method: 'PATCH',
        body: JSON.stringify(input),
      }).then((r) => r.endpoint),
    onSuccess: () => invalidate(id),
  })
}

export function useDeleteEndpoint() {
  const invalidate = useInvalidateEndpoints()
  return useMutation({
    mutationFn: (id: number) => api<void>(`/api/endpoints/${id}`, { method: 'DELETE' }),
    onSuccess: () => invalidate(),
  })
}

export function usePauseEndpoint(id: number) {
  const invalidate = useInvalidateEndpoints()
  return useMutation({
    mutationFn: (duration?: string) =>
      api<{ endpoint: Endpoint }>(`/api/endpoints/${id}/pause`, {
        method: 'POST',
        body: JSON.stringify(duration ? { duration } : {}),
      }).then((r) => r.endpoint),
    onSuccess: () => invalidate(id),
  })
}

export function useResumeEndpoint(id: number) {
  const invalidate = useInvalidateEndpoints()
  return useMutation({
    mutationFn: () =>
      api<{ endpoint: Endpoint }>(`/api/endpoints/${id}/resume`, { method: 'POST' }).then(
        (r) => r.endpoint,
      ),
    onSuccess: () => invalidate(id),
  })
}

export function useCheckNow(id: number) {
  const invalidate = useInvalidateEndpoints()
  return useMutation({
    mutationFn: () =>
      api<{ endpoint: Endpoint }>(`/api/endpoints/${id}/check`, { method: 'POST' }).then(
        (r) => r.endpoint,
      ),
    onSuccess: () => invalidate(id),
  })
}
