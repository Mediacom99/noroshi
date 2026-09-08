import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { useCreateEndpoint } from '../api/types'
import { formatInterval, parseDuration } from '../lib/format'

interface AddEndpointDialogProps {
  onDone: () => void
}

// Modal wrapper around the add-endpoint form. Mount it conditionally
// ({open && <AddEndpointDialog …/>}) so the form state resets on close.
export function AddEndpointDialog({ onDone }: AddEndpointDialogProps) {
  const [name, setName] = useState('')
  const [url, setUrl] = useState('')
  const [interval, setInterval_] = useState('1m')
  const [expectedStatus, setExpectedStatus] = useState('')
  const [expectedKeyword, setExpectedKeyword] = useState('')
  const createEndpoint = useCreateEndpoint()

  const intervalSeconds = parseDuration(interval)
  const intervalError =
    interval.trim() === ''
      ? 'Interval is required'
      : intervalSeconds === null
        ? 'Invalid duration — use e.g. 30s, 5m, 1h30m, 1d, 1w'
        : intervalSeconds < 10
          ? 'Minimum interval is 10s'
          : null

  const statusTrimmed = expectedStatus.trim()
  const statusCode = statusTrimmed === '' ? 0 : Number(statusTrimmed)
  const statusError =
    statusTrimmed !== '' && (!Number.isInteger(statusCode) || statusCode < 100 || statusCode > 599)
      ? 'Use an HTTP status code between 100 and 599, or leave empty for any 2xx'
      : null

  const valid = !intervalError && !statusError

  // Close on Escape.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onDone()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onDone])

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!name.trim() || !url.trim() || !valid || intervalSeconds === null) return
    createEndpoint.mutate(
      {
        name: name.trim(),
        url: url.trim(),
        interval_seconds: intervalSeconds,
        expected_status: statusCode,
        expected_keyword: expectedKeyword.trim(),
      },
      { onSuccess: onDone },
    )
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-black/70 p-4 backdrop-blur-sm sm:items-center"
      onClick={onDone}
    >
      <div
        className="card animate-enter w-full max-w-2xl p-5 shadow-xl"
        role="dialog"
        aria-modal="true"
        aria-label="Add endpoint"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="text-lg font-semibold text-zinc-100">Add endpoint</h2>
        <form onSubmit={handleSubmit} className="mt-4 space-y-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <label className="block">
              <span className="section-label mb-1.5 block">Name</span>
              <input
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
                placeholder="my-website"
                autoFocus
                className="input"
              />
            </label>
            <label className="block">
              <span className="section-label mb-1.5 block">Check interval</span>
              <input
                type="text"
                value={interval}
                onChange={(e) => setInterval_(e.target.value)}
                required
                placeholder="1m"
                className="input font-mono"
              />
              <p className="mt-1.5 text-xs text-zinc-500">
                Smart durations: <span className="font-mono">30s</span>,{' '}
                <span className="font-mono">5m</span>, <span className="font-mono">1h30m</span>,{' '}
                <span className="font-mono">1d</span>, <span className="font-mono">1w</span> — a
                bare number means seconds. Minimum 10s.
                {intervalSeconds !== null && !intervalError && (
                  <span className="text-zinc-400"> → every {formatInterval(intervalSeconds)}</span>
                )}
              </p>
              {intervalError && <p className="mt-1 text-xs text-rose-400">{intervalError}</p>}
            </label>
          </div>
          <label className="block">
            <span className="section-label mb-1.5 block">URL</span>
            <input
              type="text"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              required
              placeholder="https://…, tcp://host:port, dns://host, ping://host"
              className="input font-mono"
            />
          </label>
          <div className="grid gap-4 sm:grid-cols-2">
            <label className="block">
              <span className="section-label mb-1.5 block">Expected status (optional)</span>
              <input
                type="text"
                value={expectedStatus}
                onChange={(e) => setExpectedStatus(e.target.value)}
                placeholder="Any 2xx"
                inputMode="numeric"
                className="input font-mono"
              />
              <p className="mt-1.5 text-xs text-zinc-500">
                Require an exact HTTP status (e.g. <span className="font-mono">200</span>). Empty =
                any 2xx counts as up.
              </p>
              {statusError && <p className="mt-1 text-xs text-rose-400">{statusError}</p>}
            </label>
            <label className="block">
              <span className="section-label mb-1.5 block">Expected keyword (optional)</span>
              <input
                type="text"
                value={expectedKeyword}
                onChange={(e) => setExpectedKeyword(e.target.value)}
                placeholder='"status":"ok"'
                className="input font-mono"
              />
              <p className="mt-1.5 text-xs text-zinc-500">
                Body check: <span className="font-mono">text</span> must be present,{' '}
                <span className="font-mono">!text</span> must be absent,{' '}
                <span className="font-mono">re:pattern</span> regex match.
              </p>
            </label>
          </div>
          {createEndpoint.isError && (
            <p className="rounded-lg border border-rose-500/30 bg-rose-500/10 px-3 py-2 text-sm text-rose-400">
              {createEndpoint.error.message}
            </p>
          )}
          <div className="flex justify-end gap-2">
            <button type="button" onClick={onDone} className="btn btn-secondary">
              Cancel
            </button>
            <button
              type="submit"
              disabled={createEndpoint.isPending || !valid}
              className="btn btn-primary"
            >
              {createEndpoint.isPending ? 'Adding…' : 'Add endpoint'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
