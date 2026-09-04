import { useState } from 'react'
import type { FormEvent, ReactNode } from 'react'
import { createRoute, Link, useNavigate } from '@tanstack/react-router'
import { rootRoute } from './root'
import {
  useEndpoint,
  useIncidents,
  useChecks,
  useDailyStats,
  useMaintenanceWindows,
  useCheckNow,
  usePauseEndpoint,
  useResumeEndpoint,
  useUpdateEndpoint,
  useDeleteEndpoint,
} from '../api/types'
import type { Endpoint, StatsWindow, UpdateEndpointInput } from '../api/types'
import { apiOrigin } from '../api/client'
import { Header } from '../components/Header'
import { StatusDot } from '../components/StatusDot'
import { TypeChip } from '../components/TypeChip'
import { UptimeBar } from '../components/UptimeBar'
import { MaintenanceChip } from '../components/MaintenanceChip'
import { DailyUptimeStrip } from '../components/DailyUptimeStrip'
import { KpiStrip } from '../components/KpiStrip'
import { StatCard } from '../components/StatCard'
import { SegmentedControl } from '../components/SegmentedControl'
import { LatencyChart } from '../components/LatencyChart'
import { IncidentList } from '../components/IncidentList'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { formatInterval, formatLatency, formatUptime, parseDuration, relativeTime } from '../lib/format'
import { statusKind, statusTokens } from '../lib/status'
import { isEndpointInMaintenance } from '../lib/maintenance'

export const endpointDetailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/endpoints/$id',
  component: EndpointDetailPage,
})

const WINDOWS: readonly StatsWindow[] = ['24h', '7d', '30d']
const CERT_WARNING_DAYS = 14

type Tab = 'overview' | 'incidents' | 'config'

function EndpointDetailPage() {
  const { id: idParam } = endpointDetailRoute.useParams()
  const id = Number(idParam)
  const navigate = useNavigate()

  const endpointQuery = useEndpoint(id)
  const [window_, setWindow] = useState<StatsWindow>('24h')
  const checksQuery = useChecks(id, window_)
  const heroChecksQuery = useChecks(id, '24h')
  const dailyQuery = useDailyStats(id)
  const incidentsQuery = useIncidents(id)
  const maintenanceQuery = useMaintenanceWindows()

  const checkNow = useCheckNow(id)
  const pauseEndpoint = usePauseEndpoint(id)
  const resumeEndpoint = useResumeEndpoint(id)
  const updateEndpoint = useUpdateEndpoint(id)
  const deleteEndpoint = useDeleteEndpoint()

  const [tab, setTab] = useState<Tab>('overview')
  const [showPauseForm, setShowPauseForm] = useState(false)
  const [pauseDuration, setPauseDuration] = useState('')
  const [showIntervalForm, setShowIntervalForm] = useState(false)
  const [intervalValue, setIntervalValue] = useState('')
  const [showDeleteDialog, setShowDeleteDialog] = useState(false)
  const [copied, setCopied] = useState<'url' | 'markdown' | null>(null)

  function copyBadge(text: string, kind: 'url' | 'markdown') {
    void navigator.clipboard.writeText(text).then(() => {
      setCopied(kind)
      setTimeout(() => setCopied(null), 2000)
    })
  }

  if (endpointQuery.isLoading) {
    return (
      <div className="min-h-screen">
        <Header />
        <main className="mx-auto max-w-5xl px-4 py-8">
          <div className="card animate-pulse p-5">
            <div className="h-6 w-56 rounded bg-zinc-800" />
            <div className="mt-2 h-4 w-72 rounded bg-zinc-800" />
            <div className="mt-4 h-8 rounded bg-zinc-800/70" />
          </div>
        </main>
      </div>
    )
  }

  if (endpointQuery.isError || !endpointQuery.data) {
    return (
      <div className="min-h-screen">
        <Header />
        <main className="mx-auto max-w-5xl px-4 py-8">
          <BackLink />
          <div className="card mt-4 border-rose-900/60 p-4 text-sm text-rose-400">
            {endpointQuery.error?.message ?? 'Endpoint not found'}
          </div>
        </main>
      </div>
    )
  }

  const { endpoint, stats } = endpointQuery.data
  const kind = statusKind(endpoint.status, endpoint.paused)
  const tokens = statusTokens[kind]
  const certDays = certDaysRemaining(endpoint.cert_expires_at)
  const inMaintenance = isEndpointInMaintenance(maintenanceQuery.data, endpoint.id)
  const badgeUrl = `${apiOrigin()}/badge/${encodeURIComponent(endpoint.name)}.svg`
  const incidentCount = incidentsQuery.data?.length

  const tabs: { key: Tab; label: string; badge?: number }[] = [
    { key: 'overview', label: 'Overview' },
    { key: 'incidents', label: 'Incidents', badge: incidentCount },
    { key: 'config', label: 'Configuration' },
  ]

  function handlePause(e: FormEvent) {
    e.preventDefault()
    pauseEndpoint.mutate(pauseDuration.trim() || undefined, {
      onSuccess: () => {
        setShowPauseForm(false)
        setPauseDuration('')
      },
    })
  }

  function handleIntervalSubmit(e: FormEvent) {
    e.preventDefault()
    const seconds = parseDuration(intervalValue)
    if (seconds === null || seconds < 10) return
    updateEndpoint.mutate(
      { interval_seconds: seconds },
      { onSuccess: () => setShowIntervalForm(false) },
    )
  }

  const parsedInterval = parseDuration(intervalValue)
  const intervalError =
    intervalValue.trim() === ''
      ? null
      : parsedInterval === null
        ? 'Invalid duration — use e.g. 30s, 5m, 1h30m'
        : parsedInterval < 10
          ? 'Minimum interval is 10s'
          : null

  function handleDelete() {
    deleteEndpoint.mutate(id, { onSuccess: () => void navigate({ to: '/' }) })
  }

  return (
    <div className="min-h-screen">
      <Header />
      <main className="mx-auto max-w-5xl px-4 py-8">
        <BackLink />

        <section className="card mt-4 p-5">
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2.5">
                <StatusDot status={endpoint.status} paused={endpoint.paused} showLabel={false} />
                <h1 className="truncate text-xl font-semibold tracking-tight">{endpoint.name}</h1>
                <TypeChip type={endpoint.type} />
                {inMaintenance && <MaintenanceChip />}
                {certDays !== null && certDays <= CERT_WARNING_DAYS && (
                  <span className="rounded-full border border-amber-500/30 bg-amber-500/10 px-2 py-0.5 text-[11px] font-medium text-amber-400">
                    {certDays <= 0 ? 'Certificate expired' : `Cert expires in ${certDays}d`}
                  </span>
                )}
              </div>
              <p className="mt-1 truncate font-mono text-sm text-zinc-500">{endpoint.url}</p>
              <p className="mt-2 text-sm">
                <span className={`font-medium ${tokens.text}`}>{tokens.label}</span>
                <span className="text-zinc-500">
                  {' '}
                  · last checked {relativeTime(endpoint.last_checked_at)} · every{' '}
                  {formatInterval(endpoint.interval_seconds)}
                </span>
                {endpoint.paused && endpoint.paused_until && (
                  <span className="text-zinc-500">
                    {' '}
                    · paused until {new Date(endpoint.paused_until).toLocaleString()}
                  </span>
                )}
              </p>
            </div>

            <div className="flex flex-wrap items-center gap-2">
              <button
                onClick={() => checkNow.mutate()}
                disabled={endpoint.paused || checkNow.isPending}
                className="btn btn-primary"
              >
                {checkNow.isPending ? 'Checking…' : 'Check now'}
              </button>
              {endpoint.paused ? (
                <button
                  onClick={() => resumeEndpoint.mutate()}
                  disabled={resumeEndpoint.isPending}
                  className="btn btn-secondary"
                >
                  {resumeEndpoint.isPending ? 'Resuming…' : 'Resume'}
                </button>
              ) : (
                <button
                  onClick={() => setShowPauseForm((v) => !v)}
                  className="btn btn-secondary"
                >
                  Pause
                </button>
              )}
              <button
                onClick={() => {
                  setIntervalValue(formatInterval(endpoint.interval_seconds))
                  setShowIntervalForm((v) => !v)
                }}
                className="btn btn-secondary"
              >
                Interval
              </button>
              <button
                onClick={() => void navigate({ to: '/', search: { clone: String(id) } })}
                className="btn btn-secondary"
              >
                Clone
              </button>
              <button onClick={() => setShowDeleteDialog(true)} className="btn btn-danger">
                Delete
              </button>
            </div>
          </div>

          <div className="mt-5">
            <UptimeBar
              checks={heroChecksQuery.data}
              paused={endpoint.paused}
              barHeight="h-8"
            />
            <div className="mt-1.5 flex justify-between text-[11px] text-zinc-600">
              <span>24h ago</span>
              <span>now</span>
            </div>
          </div>

          {showPauseForm && !endpoint.paused && (
            <form
              onSubmit={handlePause}
              className="mt-4 flex flex-wrap items-center gap-2 rounded-lg border border-zinc-800/60 bg-zinc-950/60 p-3"
            >
              <input
                type="text"
                value={pauseDuration}
                onChange={(e) => setPauseDuration(e.target.value)}
                placeholder="Duration (e.g. 2h, empty = indefinite)"
                className="input w-64"
              />
              <button type="submit" disabled={pauseEndpoint.isPending} className="btn btn-primary">
                {pauseEndpoint.isPending ? 'Pausing…' : 'Pause'}
              </button>
              <button
                type="button"
                onClick={() => setShowPauseForm(false)}
                className="btn btn-secondary"
              >
                Cancel
              </button>
            </form>
          )}

          {showIntervalForm && (
            <form
              onSubmit={handleIntervalSubmit}
              className="mt-4 rounded-lg border border-zinc-800/60 bg-zinc-950/60 p-3"
            >
              <div className="flex flex-wrap items-center gap-2">
                <input
                  type="text"
                  value={intervalValue}
                  onChange={(e) => setIntervalValue(e.target.value)}
                  placeholder="e.g. 30s, 5m, 1h30m"
                  className="input w-48"
                />
                <button
                  type="submit"
                  disabled={
                    updateEndpoint.isPending || parsedInterval === null || parsedInterval < 10
                  }
                  className="btn btn-primary"
                >
                  {updateEndpoint.isPending ? 'Saving…' : 'Save'}
                </button>
                <button
                  type="button"
                  onClick={() => setShowIntervalForm(false)}
                  className="btn btn-secondary"
                >
                  Cancel
                </button>
                {updateEndpoint.isError && (
                  <span className="text-xs text-rose-400">{updateEndpoint.error.message}</span>
                )}
              </div>
              <p className="mt-1.5 text-xs text-zinc-500">
                Format: 10s, 30s, 5m, 1h — minimum 10s
              </p>
              {intervalError && <p className="mt-1 text-xs text-rose-400">{intervalError}</p>}
            </form>
          )}

          {checkNow.isSuccess && (
            <p className="mt-4 text-sm text-zinc-400">
              Check finished:{' '}
              {checkNow.data.status === 'ok' ? (
                <span className="text-emerald-400">
                  up ({formatLatency(checkNow.data.last_latency_ms)})
                </span>
              ) : (
                <span className="text-rose-400">
                  down
                  {checkNow.data.last_check_error ? ` — ${checkNow.data.last_check_error}` : ''}
                </span>
              )}
            </p>
          )}
          {checkNow.isError && (
            <p className="mt-4 text-sm text-rose-400">{checkNow.error.message}</p>
          )}

          <div className="mt-5 border-t border-zinc-800/60 pt-4">
            <p className="section-label">Status badge</p>
            <div className="mt-2 flex flex-wrap items-center gap-3">
              <img src={badgeUrl} alt={`Status badge for ${endpoint.name}`} className="h-5" />
              <code className="min-w-0 flex-1 truncate font-mono text-xs text-zinc-500">
                {badgeUrl}
              </code>
              <button
                onClick={() => copyBadge(badgeUrl, 'url')}
                className="btn btn-secondary px-2 py-0.5 text-xs"
              >
                {copied === 'url' ? 'Copied' : 'Copy URL'}
              </button>
              <button
                onClick={() => copyBadge(`![status](${badgeUrl})`, 'markdown')}
                className="btn btn-secondary px-2 py-0.5 text-xs"
              >
                {copied === 'markdown' ? 'Copied' : 'Markdown'}
              </button>
            </div>
          </div>
        </section>

        <nav className="mt-6 border-b border-zinc-800/60" aria-label="Endpoint sections">
          <div className="flex gap-6">
            {tabs.map((t) => (
              <button
                key={t.key}
                onClick={() => setTab(t.key)}
                aria-current={tab === t.key ? 'page' : undefined}
                className={`-mb-px cursor-pointer border-b-2 pb-3 text-sm font-medium transition-colors duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-400/40 ${
                  tab === t.key
                    ? 'border-emerald-500 text-zinc-100'
                    : 'border-transparent text-zinc-400 hover:text-zinc-200'
                }`}
              >
                {t.label}
                {t.badge !== undefined && (
                  <span className="ml-1.5 rounded-full bg-zinc-800 px-1.5 py-0.5 text-[10px] tabular-nums text-zinc-400">
                    {t.badge}
                  </span>
                )}
              </button>
            ))}
          </div>
        </nav>

        {tab === 'overview' && (
          <>
            <section className="mt-6 grid gap-3 sm:grid-cols-3">
              {WINDOWS.map((w) => (
                <StatCard key={w} label={w} stats={stats[w]} />
              ))}
            </section>

            <section className="mt-8">
              <h2 className="section-label">Uptime</h2>
              <div className="card mt-3 p-4">
                <DailyUptimeStrip days={dailyQuery.data} />
                <div className="mt-2 flex justify-between text-[11px] text-zinc-600">
                  <span>Last 30 days</span>
                  <span className="tabular-nums">
                    {stats['30d'] ? `${formatUptime(stats['30d'].uptime)}% uptime` : ''}
                  </span>
                </div>
              </div>
            </section>

            <section className="mt-8">
              <h2 className="section-label">Analytics</h2>
              <div className="mt-3">
                <KpiStrip checks={checksQuery.data ?? []} incidents={incidentsQuery.data ?? []} />
              </div>
            </section>

            <section className="mt-8">
              <div className="flex items-center justify-between">
                <h2 className="section-label">Latency</h2>
                <SegmentedControl
                  options={WINDOWS}
                  value={window_}
                  onChange={setWindow}
                  ariaLabel="Latency window"
                />
              </div>
              <div className="mt-3">
                {checksQuery.isLoading ? (
                  <div className="card flex h-44 items-center justify-center">
                    <div className="h-4 w-32 animate-pulse rounded bg-zinc-800" />
                  </div>
                ) : checksQuery.isError ? (
                  <div className="card flex h-44 items-center justify-center text-sm text-rose-400">
                    {checksQuery.error.message}
                  </div>
                ) : (
                  <LatencyChart checks={checksQuery.data ?? []} window={window_} />
                )}
              </div>
            </section>
          </>
        )}

        {tab === 'incidents' && (
          <section className="mt-6">
            {incidentsQuery.isLoading ? (
              <p className="text-sm text-zinc-500">Loading incidents…</p>
            ) : incidentsQuery.isError ? (
              <p className="text-sm text-rose-400">{incidentsQuery.error.message}</p>
            ) : (
              <IncidentList incidents={incidentsQuery.data ?? []} relaxed />
            )}
          </section>
        )}

        {tab === 'config' && (
          <section className="mt-6">
            <div className="card space-y-3 p-4 text-sm">
              <RenameRow endpoint={endpoint} />
              <p>
                <span className="text-zinc-500">Interval </span>
                <span className="tabular-nums text-zinc-200">
                  {formatInterval(endpoint.interval_seconds)}
                </span>
              </p>
              <ExpectedStatusRow endpoint={endpoint} />
              <ExpectedKeywordRow endpoint={endpoint} />
              <p>
                <span className="text-zinc-500">Created </span>
                <span className="text-zinc-200">
                  {new Date(endpoint.created_at).toLocaleDateString()}
                </span>
              </p>
              <p>
                <span className="text-zinc-500">Last checked </span>
                <span className="text-zinc-200">{relativeTime(endpoint.last_checked_at)}</span>
              </p>
              {endpoint.last_checked_at && !endpoint.paused && (
                <p>
                  <span className="text-zinc-500">Last latency </span>
                  <span className="tabular-nums text-zinc-200">
                    {formatLatency(endpoint.last_latency_ms)}
                  </span>
                  {endpoint.last_status_code > 0 && (
                    <span className="ml-3 text-zinc-500">HTTP {endpoint.last_status_code}</span>
                  )}
                </p>
              )}
              {endpoint.last_check_error && endpoint.status === 'not_ok' && (
                <p>
                  <span className="text-zinc-500">Last error </span>
                  <span className="text-rose-400">{endpoint.last_check_error}</span>
                </p>
              )}
            </div>
          </section>
        )}
      </main>

      <ConfirmDialog
        open={showDeleteDialog}
        title="Delete endpoint"
        message={`Delete "${endpoint.name}"? This will remove the endpoint and its check history. This cannot be undone.`}
        confirmLabel="Delete"
        pending={deleteEndpoint.isPending}
        onConfirm={handleDelete}
        onCancel={() => setShowDeleteDialog(false)}
      />
    </div>
  )
}

function BackLink() {
  return (
    <Link
      to="/"
      className="rounded-md text-sm text-zinc-500 transition-colors duration-150 hover:text-zinc-300 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-400/40"
    >
      &larr; Back to dashboard
    </Link>
  )
}

function certDaysRemaining(certExpiresAt: string | null): number | null {
  if (!certExpiresAt) return null
  const expires = new Date(certExpiresAt).getTime()
  if (Number.isNaN(expires)) return null
  return Math.ceil((expires - Date.now()) / (24 * 60 * 60 * 1000))
}

// Generic inline-edit row for the Configuration tab: label + value + Edit
// button, expanding into an input row with Save/Cancel, client-side
// validation and inline API error display.
function EditableRow({
  endpoint,
  label,
  display,
  initialValue,
  placeholder,
  hint,
  mono = false,
  validate,
  buildPatch,
}: {
  endpoint: Endpoint
  label: string
  display: ReactNode
  initialValue: string
  placeholder?: string
  hint?: string
  mono?: boolean
  validate?: (value: string) => string | null
  buildPatch: (value: string) => UpdateEndpointInput
}) {
  const [editing, setEditing] = useState(false)
  const [value, setValue] = useState('')
  const update = useUpdateEndpoint(endpoint.id)
  const validationError = validate ? validate(value) : null

  function submit(e: FormEvent) {
    e.preventDefault()
    if (validationError !== null) return
    update.mutate(buildPatch(value), { onSuccess: () => setEditing(false) })
  }

  if (!editing) {
    return (
      <div className="flex flex-wrap items-center gap-3">
        <span className="text-zinc-500">{label}</span>
        <span className="text-zinc-200">{display}</span>
        <button
          onClick={() => {
            setValue(initialValue)
            setEditing(true)
          }}
          className="btn btn-secondary px-2 py-0.5 text-xs"
        >
          Edit
        </button>
      </div>
    )
  }

  return (
    <form onSubmit={submit}>
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-zinc-500">{label}</span>
        <input
          type="text"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          placeholder={placeholder}
          autoFocus
          className={`input w-64 ${mono ? 'font-mono' : ''}`}
        />
        <button
          type="submit"
          disabled={update.isPending || validationError !== null}
          className="btn btn-primary"
        >
          {update.isPending ? 'Saving…' : 'Save'}
        </button>
        <button type="button" onClick={() => setEditing(false)} className="btn btn-secondary">
          Cancel
        </button>
        {update.isError && (
          <span className="text-xs text-rose-400">{update.error.message}</span>
        )}
      </div>
      {hint && <p className="mt-1.5 text-xs text-zinc-500">{hint}</p>}
      {validationError && <p className="mt-1 text-xs text-rose-400">{validationError}</p>}
    </form>
  )
}

function RenameRow({ endpoint }: { endpoint: Endpoint }) {
  return (
    <EditableRow
      endpoint={endpoint}
      label="Name"
      display={endpoint.name}
      initialValue={endpoint.name}
      validate={(v) => (v.trim() === '' ? 'Name is required' : null)}
      buildPatch={(v) => ({ name: v.trim() })}
    />
  )
}

function parseExpectedStatus(value: string): number | null {
  const t = value.trim().toLowerCase()
  if (t === 'any') return 0
  if (!/^\d+$/.test(t)) return null
  const n = Number(t)
  return n >= 100 && n <= 599 ? n : null
}

function ExpectedStatusRow({ endpoint }: { endpoint: Endpoint }) {
  return (
    <EditableRow
      endpoint={endpoint}
      label="Expected status"
      display={endpoint.expected_status === 0 ? 'Any 2xx' : String(endpoint.expected_status)}
      initialValue={endpoint.expected_status === 0 ? 'any' : String(endpoint.expected_status)}
      placeholder="200 or any"
      hint="A status code 100–599, or 'any' to accept any 2xx"
      validate={(v) =>
        parseExpectedStatus(v) === null ? "Enter a status code 100–599, or 'any'" : null
      }
      buildPatch={(v) => ({ expected_status: parseExpectedStatus(v) ?? 0 })}
    />
  )
}

function ExpectedKeywordRow({ endpoint }: { endpoint: Endpoint }) {
  return (
    <EditableRow
      endpoint={endpoint}
      label="Expected keyword"
      display={
        endpoint.expected_keyword ? (
          <span className="font-mono">{endpoint.expected_keyword}</span>
        ) : (
          '—'
        )
      }
      initialValue={endpoint.expected_keyword}
      placeholder="e.g. ok, !error, re:up|down"
      hint="text, !text, re:pattern, !re:pattern — empty clears"
      mono
      buildPatch={(v) => ({ expected_keyword: v.trim() })}
    />
  )
}
