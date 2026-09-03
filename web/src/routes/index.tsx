import { useState } from 'react'
import { createRoute, Link, useNavigate } from '@tanstack/react-router'
import { rootRoute } from './root'
import { clearToken } from '../api/client'
import { useChecks24h, useDailyStatsAll, useEndpoints } from '../api/types'
import type { Check, Endpoint } from '../api/types'
import { Header } from '../components/Header'
import { StatusDot } from '../components/StatusDot'
import { TypeChip } from '../components/TypeChip'
import { UptimeBar } from '../components/UptimeBar'
import { AddEndpointForm } from '../components/AddEndpointForm'
import { formatLatency, formatUptime, relativeTime } from '../lib/format'
import { statusKind, statusTokens, uptimeTextColor } from '../lib/status'
import type { StatusKind } from '../lib/status'
import { overallUptime } from '../lib/stats'
import type { DayStat } from '../api/types'

export const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  component: DashboardPage,
})

type Filter = 'all' | 'up' | 'down' | 'paused'

// Sort order: attention first — down, unknown, up, paused; alphabetical within.
const SORT_ORDER: Record<StatusKind, number> = { down: 0, unknown: 1, up: 2, paused: 3 }

function DashboardPage() {
  const navigate = useNavigate()
  const { data: endpoints, isLoading, isError, error, dataUpdatedAt } = useEndpoints()
  const checksResults = useChecks24h(endpoints)
  const dailyResults = useDailyStatsAll(endpoints)
  const [filter, setFilter] = useState<Filter>('all')
  const [query, setQuery] = useState('')
  const [showAddForm, setShowAddForm] = useState(false)

  function logout() {
    clearToken()
    void navigate({ to: '/login' })
  }

  const counts = { up: 0, down: 0, paused: 0, unknown: 0 }
  for (const e of endpoints ?? []) {
    counts[statusKind(e.status, e.paused)]++
  }
  const total = endpoints?.length ?? 0

  const rows = (endpoints ?? [])
    .map((endpoint, i) => ({
      endpoint,
      checks: checksResults[i]?.data,
      days: dailyResults[i]?.data,
    }))
    .sort((a, b) => {
      const byStatus =
        SORT_ORDER[statusKind(a.endpoint.status, a.endpoint.paused)] -
        SORT_ORDER[statusKind(b.endpoint.status, b.endpoint.paused)]
      return byStatus !== 0 ? byStatus : a.endpoint.name.localeCompare(b.endpoint.name)
    })

  const needle = query.trim().toLowerCase()
  const filteredRows = rows.filter(({ endpoint }) => {
    if (filter !== 'all' && statusKind(endpoint.status, endpoint.paused) !== filter) return false
    if (!needle) return true
    return (
      endpoint.name.toLowerCase().includes(needle) || endpoint.url.toLowerCase().includes(needle)
    )
  })

  const filters: { key: Filter; label: string; count: number }[] = [
    { key: 'all', label: 'All', count: total },
    { key: 'up', label: 'Up', count: counts.up },
    { key: 'down', label: 'Down', count: counts.down },
    { key: 'paused', label: 'Paused', count: counts.paused },
  ]

  return (
    <div className="min-h-screen">
      <Header
        right={
          <div className="flex items-center gap-4">
            <LiveIndicator dataUpdatedAt={dataUpdatedAt} isError={isError} />
            <button onClick={logout} className="btn btn-secondary">
              Log out
            </button>
          </div>
        }
      />

      <main className="mx-auto max-w-5xl px-4 py-8">
        {isLoading && <SkeletonRows />}

        {isError && (
          <div className="card border-rose-900/60 p-4 text-sm text-rose-400">{error.message}</div>
        )}

        {endpoints && (
          <>
            <StatusBanner counts={counts} total={total} />

            {total > 0 && (
              <div className="mt-6 flex flex-wrap items-center gap-2">
                <input
                  type="search"
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                  placeholder="Filter by name or URL…"
                  aria-label="Filter endpoints"
                  className="input w-full sm:w-64"
                />
                {filters.map((f) => (
                  <button
                    key={f.key}
                    onClick={() => setFilter(f.key)}
                    aria-pressed={filter === f.key}
                    className={`chip ${
                      filter === f.key
                        ? 'border-zinc-600 bg-zinc-800 text-zinc-100'
                        : 'border-zinc-800/60 text-zinc-500 hover:border-zinc-700 hover:text-zinc-300'
                    }`}
                  >
                    {f.label}
                    <span className="tabular-nums text-zinc-500">{f.count}</span>
                  </button>
                ))}
              </div>
            )}

            <div className="mt-4 space-y-3">
              {total === 0 && (
                <div className="card p-10 text-center">
                  <p className="text-sm font-medium text-zinc-300">No endpoints yet</p>
                  <p className="mt-1 text-sm text-zinc-500">
                    Add your first endpoint to start monitoring uptime and latency.
                  </p>
                </div>
              )}
              {total > 0 && filteredRows.length === 0 && (
                <div className="card p-6 text-center text-sm text-zinc-500">
                  No endpoints match your filters.
                </div>
              )}
              {filteredRows.map(({ endpoint, checks, days }) => (
                <EndpointRow
                  key={endpoint.id}
                  endpoint={endpoint}
                  checks={checks}
                  days={days}
                />
              ))}
            </div>
          </>
        )}

        <div className="mt-6">
          {showAddForm ? (
            <AddEndpointForm onDone={() => setShowAddForm(false)} />
          ) : (
            <button onClick={() => setShowAddForm(true)} className="btn btn-secondary">
              Add endpoint
            </button>
          )}
        </div>
      </main>
    </div>
  )
}

function LiveIndicator({
  dataUpdatedAt,
  isError,
}: {
  dataUpdatedAt: number
  isError: boolean
}) {
  if (isError) {
    return (
      <span className="flex items-center gap-1.5 text-xs text-zinc-500">
        <span className="h-1.5 w-1.5 rounded-full bg-zinc-600" />
        Offline
      </span>
    )
  }
  return (
    <span className="flex items-center gap-1.5 text-xs text-zinc-400">
      <span className="relative flex h-1.5 w-1.5">
        <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-60" />
        <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-emerald-500" />
      </span>
      Live
      {dataUpdatedAt > 0 && (
        <span className="text-zinc-600">
          · updated {relativeTime(new Date(dataUpdatedAt).toISOString())}
        </span>
      )}
    </span>
  )
}

function StatusBanner({ counts, total }: { counts: Record<StatusKind, number>; total: number }) {
  const kind: StatusKind =
    total === 0
      ? 'unknown'
      : counts.down > 0
        ? 'down'
        : counts.up === total
          ? 'up'
          : counts.unknown > 0
            ? 'unknown'
            : 'paused'
  const token = statusTokens[kind]
  const title =
    kind === 'down'
      ? 'Partial outage'
      : kind === 'up'
        ? 'All systems operational'
        : kind === 'paused'
          ? 'Some endpoints paused'
          : total === 0
            ? 'Nothing monitored yet'
            : 'Some endpoints have unknown status'

  return (
    <div className="card flex items-center gap-3 p-4">
      <span className={`h-3 w-3 shrink-0 rounded-full ${token.dot}`} />
      <div>
        <p className={`text-sm font-semibold ${token.text}`}>{title}</p>
        <p className="mt-0.5 text-xs tabular-nums text-zinc-500">
          {counts.up} up · {counts.down} down · {counts.paused} paused
          {counts.unknown > 0 ? ` · ${counts.unknown} unknown` : ''}
        </p>
      </div>
    </div>
  )
}

function EndpointRow({
  endpoint,
  checks,
  days,
}: {
  endpoint: Endpoint
  checks: Check[] | undefined
  days: DayStat[] | undefined
}) {
  const uptime30d = overallUptime(days)

  return (
    <Link
      to="/endpoints/$id"
      params={{ id: String(endpoint.id) }}
      className="card group/row block p-4 transition-colors duration-150 hover:border-zinc-700/80 hover:bg-zinc-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-400/40"
    >
      <div className="flex items-center gap-3">
        <StatusDot status={endpoint.status} paused={endpoint.paused} showLabel={false} />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="truncate text-sm font-medium text-zinc-100 group-hover/row:text-white">
              {endpoint.name}
            </span>
            <TypeChip type={endpoint.type} />
          </div>
          <p className="mt-0.5 truncate font-mono text-xs text-zinc-500">{endpoint.url}</p>
        </div>
        <div className="hidden shrink-0 text-right sm:block">
          {uptime30d !== null && (
            <p className={`text-sm font-medium tabular-nums ${uptimeTextColor(uptime30d)}`}>
              {formatUptime(uptime30d)}%{' '}
              <span className="text-[10px] font-normal text-zinc-500">30d</span>
            </p>
          )}
          {endpoint.last_checked_at && !endpoint.paused && (
            <p className="mt-0.5 text-xs tabular-nums text-zinc-400">
              {formatLatency(endpoint.last_latency_ms)}
            </p>
          )}
          <p className="mt-0.5 text-xs text-zinc-500">
            {endpoint.paused ? 'paused' : `checked ${relativeTime(endpoint.last_checked_at)}`}
          </p>
        </div>
      </div>
      <div className="mt-3">
        <UptimeBar checks={checks} paused={endpoint.paused} />
      </div>
    </Link>
  )
}

function SkeletonRows() {
  return (
    <div className="space-y-3" aria-label="Loading endpoints">
      <div className="card h-[68px] animate-pulse p-4">
        <div className="h-4 w-48 rounded bg-zinc-800" />
      </div>
      {[0, 1, 2].map((i) => (
        <div key={i} className="card animate-pulse p-4">
          <div className="flex items-center gap-3">
            <div className="h-2.5 w-2.5 rounded-full bg-zinc-800" />
            <div className="h-4 w-40 rounded bg-zinc-800" />
            <div className="ml-auto h-4 w-16 rounded bg-zinc-800" />
          </div>
          <div className="mt-3 h-6 rounded bg-zinc-800/70" />
        </div>
      ))}
    </div>
  )
}
