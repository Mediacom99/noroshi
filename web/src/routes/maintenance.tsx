import { useState } from 'react'
import type { FormEvent } from 'react'
import { createRoute, useNavigate } from '@tanstack/react-router'
import { rootRoute } from './root'
import { clearToken } from '../api/client'
import {
  useAddMaintenanceWindow,
  useDeleteMaintenanceWindow,
  useEndpoints,
  useMaintenanceWindows,
} from '../api/types'
import type { MaintenanceWindow } from '../api/types'
import { Header } from '../components/Header'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { DAY_CODES, formatDays, minutesToTime } from '../lib/maintenance'

export const maintenanceRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/maintenance',
  component: MaintenancePage,
})

const DAY_CHIP_LABELS: Record<string, string> = {
  mon: 'Mon',
  tue: 'Tue',
  wed: 'Wed',
  thu: 'Thu',
  fri: 'Fri',
  sat: 'Sat',
  sun: 'Sun',
}

function MaintenancePage() {
  const navigate = useNavigate()
  const endpointsQuery = useEndpoints()
  const windowsQuery = useMaintenanceWindows()
  const deleteWindow = useDeleteMaintenanceWindow()
  const [showAddForm, setShowAddForm] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<MaintenanceWindow | null>(null)

  function logout() {
    clearToken()
    void navigate({ to: '/login' })
  }

  const endpointName = (id: number | null) => {
    if (id === null) return 'All endpoints'
    return endpointsQuery.data?.find((e) => e.id === id)?.name ?? `Endpoint #${id}`
  }

  const windows = windowsQuery.data

  return (
    <div className="min-h-screen">
      <Header
        right={
          <button onClick={logout} className="btn btn-secondary">
            Log out
          </button>
        }
      />

      <main className="mx-auto max-w-5xl px-4 py-8">
        <div className="flex items-center justify-between">
          <h1 className="text-xl font-semibold tracking-tight">Maintenance windows</h1>
          {!showAddForm && (
            <button onClick={() => setShowAddForm(true)} className="btn btn-secondary">
              Add window
            </button>
          )}
        </div>
        <p className="mt-1 text-sm text-zinc-500">
          During a maintenance window, checks are paused and no alerts fire. All times are UTC.
        </p>

        {showAddForm && <AddWindowForm onDone={() => setShowAddForm(false)} />}

        <div className="mt-6">
          {windowsQuery.isLoading && (
            <div className="space-y-3" aria-label="Loading maintenance windows">
              {[0, 1].map((i) => (
                <div key={i} className="card animate-pulse p-4">
                  <div className="h-4 w-56 rounded bg-zinc-800" />
                  <div className="mt-2 h-3 w-32 rounded bg-zinc-800" />
                </div>
              ))}
            </div>
          )}
          {windowsQuery.isError && (
            <div className="card border-rose-900/60 p-4 text-sm text-rose-400">
              {windowsQuery.error.message}
            </div>
          )}
          {windows && windows.length === 0 && (
            <div className="card p-10 text-center">
              <p className="text-sm font-medium text-zinc-300">No maintenance windows</p>
              <p className="mt-1 text-sm text-zinc-500">
                Checks run around the clock. Add a window to silence recurring maintenance.
              </p>
            </div>
          )}
          {windows && windows.length > 0 && (
            <ul className="space-y-3">
              {windows.map((w) => (
                <li key={w.id} className="card flex items-center gap-4 p-4">
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="truncate text-sm font-medium text-zinc-100">
                        {endpointName(w.endpoint_id)}
                      </span>
                      {w.active && (
                        <span className="rounded-full border border-amber-500/30 bg-amber-500/10 px-2 py-0.5 text-[11px] font-medium text-amber-400">
                          Active now
                        </span>
                      )}
                    </div>
                    <p className="mt-1 text-xs text-zinc-500">
                      {formatDays(w.days)} ·{' '}
                      <span className="tabular-nums">
                        {minutesToTime(w.start_minutes)}–{minutesToTime(w.end_minutes)} UTC
                      </span>
                    </p>
                  </div>
                  <button
                    onClick={() => setDeleteTarget(w)}
                    className="btn btn-danger px-2 py-1 text-xs"
                  >
                    Delete
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      </main>

      <ConfirmDialog
        open={deleteTarget !== null}
        title="Delete maintenance window"
        message={
          deleteTarget
            ? `Delete the window for ${endpointName(deleteTarget.endpoint_id)} (${formatDays(deleteTarget.days)}, ${minutesToTime(deleteTarget.start_minutes)}–${minutesToTime(deleteTarget.end_minutes)} UTC)?`
            : ''
        }
        confirmLabel="Delete"
        pending={deleteWindow.isPending}
        onConfirm={() =>
          deleteTarget &&
          deleteWindow.mutate(deleteTarget.id, { onSuccess: () => setDeleteTarget(null) })
        }
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  )
}

function AddWindowForm({ onDone }: { onDone: () => void }) {
  const endpointsQuery = useEndpoints()
  const addWindow = useAddMaintenanceWindow()
  const [endpointId, setEndpointId] = useState('')
  const [everyDay, setEveryDay] = useState(true)
  const [days, setDays] = useState<Set<string>>(new Set())
  const [start, setStart] = useState('02:00')
  const [end, setEnd] = useState('04:00')
  const [validationError, setValidationError] = useState<string | null>(null)

  function toggleDay(day: string) {
    setDays((prev) => {
      const next = new Set(prev)
      if (next.has(day)) next.delete(day)
      else next.add(day)
      return next
    })
  }

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!start || !end || start === end) {
      setValidationError('Start and end times must be set and differ.')
      return
    }
    if (!everyDay && days.size === 0) {
      setValidationError('Select at least one day, or choose Every day.')
      return
    }
    setValidationError(null)
    const dayString = everyDay
      ? 'all'
      : DAY_CODES.filter((d) => days.has(d)).join(',')
    addWindow.mutate(
      {
        endpoint_id: endpointId === '' ? null : Number(endpointId),
        days: dayString,
        start,
        end,
      },
      { onSuccess: onDone },
    )
  }

  return (
    <form onSubmit={handleSubmit} className="card animate-enter mt-4 space-y-4 p-4">
      <div className="grid gap-4 sm:grid-cols-3">
        <label className="block">
          <span className="section-label mb-1.5 block">Endpoint</span>
          <select
            value={endpointId}
            onChange={(e) => setEndpointId(e.target.value)}
            className="input"
          >
            <option value="">All endpoints</option>
            {(endpointsQuery.data ?? []).map((endpoint) => (
              <option key={endpoint.id} value={endpoint.id}>
                {endpoint.name}
              </option>
            ))}
          </select>
        </label>
        <label className="block">
          <span className="section-label mb-1.5 block">Start (UTC)</span>
          <input
            type="time"
            value={start}
            onChange={(e) => setStart(e.target.value)}
            required
            className="input"
          />
        </label>
        <label className="block">
          <span className="section-label mb-1.5 block">End (UTC)</span>
          <input
            type="time"
            value={end}
            onChange={(e) => setEnd(e.target.value)}
            required
            className="input"
          />
        </label>
      </div>

      <div>
        <span className="section-label mb-1.5 block">Days</span>
        <div className="flex flex-wrap gap-2">
          <button
            type="button"
            onClick={() => setEveryDay(true)}
            aria-pressed={everyDay}
            className={`chip ${
              everyDay
                ? 'border-zinc-600 bg-zinc-800 text-zinc-100'
                : 'border-zinc-800/60 text-zinc-500 hover:border-zinc-700 hover:text-zinc-300'
            }`}
          >
            Every day
          </button>
          {DAY_CODES.map((day) => (
            <button
              key={day}
              type="button"
              onClick={() => {
                setEveryDay(false)
                toggleDay(day)
              }}
              aria-pressed={!everyDay && days.has(day)}
              className={`chip ${
                !everyDay && days.has(day)
                  ? 'border-zinc-600 bg-zinc-800 text-zinc-100'
                  : 'border-zinc-800/60 text-zinc-500 hover:border-zinc-700 hover:text-zinc-300'
              }`}
            >
              {DAY_CHIP_LABELS[day]}
            </button>
          ))}
        </div>
        <p className="mt-2 text-xs text-zinc-500">
          Times are UTC. An end before the start means an overnight window.
        </p>
      </div>

      {validationError && <p className="text-xs text-rose-400">{validationError}</p>}
      {addWindow.isError && (
        <p className="rounded-lg border border-rose-500/30 bg-rose-500/10 px-3 py-2 text-sm text-rose-400">
          {addWindow.error.message}
        </p>
      )}

      <div className="flex justify-end gap-2">
        <button type="button" onClick={onDone} className="btn btn-secondary">
          Cancel
        </button>
        <button type="submit" disabled={addWindow.isPending} className="btn btn-primary">
          {addWindow.isPending ? 'Adding…' : 'Add window'}
        </button>
      </div>
    </form>
  )
}
