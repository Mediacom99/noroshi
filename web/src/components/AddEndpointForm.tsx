import { useState } from 'react'
import type { FormEvent } from 'react'
import { useCreateEndpoint } from '../api/types'

interface AddEndpointFormProps {
  onDone: () => void
  initial?: { name: string; url: string; interval: string }
}

export function AddEndpointForm({ onDone, initial }: AddEndpointFormProps) {
  const [name, setName] = useState(initial?.name ?? '')
  const [url, setUrl] = useState(initial?.url ?? '')
  const [interval, setInterval_] = useState(initial?.interval ?? '60')
  const createEndpoint = useCreateEndpoint()

  const intervalSeconds = Number(interval)
  const intervalValid = Number.isInteger(intervalSeconds) && intervalSeconds >= 10

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!name.trim() || !url.trim() || !intervalValid) return
    createEndpoint.mutate(
      { name: name.trim(), url: url.trim(), interval_seconds: intervalSeconds },
      { onSuccess: onDone },
    )
  }

  return (
    <form onSubmit={handleSubmit} className="card animate-enter space-y-4 p-4">
      <div className="grid gap-4 sm:grid-cols-2">
        <label className="block">
          <span className="section-label mb-1.5 block">Name</span>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            placeholder="My website"
            className="input"
          />
        </label>
        <label className="block">
          <span className="section-label mb-1.5 block">Interval (seconds)</span>
          <input
            type="number"
            value={interval}
            onChange={(e) => setInterval_(e.target.value)}
            min={10}
            step={1}
            required
            className="input"
          />
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
      {!intervalValid && (
        <p className="text-xs text-amber-400">Interval must be at least 10 seconds.</p>
      )}
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
          disabled={createEndpoint.isPending || !intervalValid}
          className="btn btn-primary"
        >
          {createEndpoint.isPending ? 'Adding…' : 'Add endpoint'}
        </button>
      </div>
    </form>
  )
}
