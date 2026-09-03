import type { Stats } from '../api/types'
import { formatLatency, formatUptime } from '../lib/format'
import { uptimeTextColor } from '../lib/status'

interface StatCardProps {
  label: string
  stats: Stats | undefined
}

export function StatCard({ label, stats }: StatCardProps) {
  return (
    <div className="card p-4">
      <p className="section-label">{label}</p>
      {stats ? (
        <>
          <p className={`mt-2 text-2xl font-semibold tabular-nums ${uptimeTextColor(stats.uptime)}`}>
            {formatUptime(stats.uptime)}%
          </p>
          <dl className="mt-3 space-y-1.5 text-xs">
            <div className="flex justify-between">
              <dt className="text-zinc-500">Avg latency</dt>
              <dd className="tabular-nums text-zinc-300">{formatLatency(stats.avg_latency_ms)}</dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-zinc-500">P95 latency</dt>
              <dd className="tabular-nums text-zinc-300">{formatLatency(stats.p95_latency_ms)}</dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-zinc-500">Incidents</dt>
              <dd className="tabular-nums text-zinc-300">{stats.incidents}</dd>
            </div>
          </dl>
        </>
      ) : (
        <p className="mt-2 text-sm text-zinc-500">No data</p>
      )}
    </div>
  )
}
