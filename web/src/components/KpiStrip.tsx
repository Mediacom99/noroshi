import type { Check, Incident } from '../api/types'
import { formatDuration, formatLatency } from '../lib/format'
import { percentile } from '../lib/stats'

interface KpiStripProps {
  checks: Check[]
  incidents: Incident[]
}

function KpiTile({ label, value }: { label: string; value: string }) {
  return (
    <div className="card p-3">
      <p className="section-label">{label}</p>
      <p className="mt-1.5 text-lg font-semibold tabular-nums text-zinc-100">{value}</p>
    </div>
  )
}

// Client-side analytics: latency stats from the visible window's checks,
// MTTR/downtime from the (window-independent) recent incident sample.
export function KpiStrip({ checks, incidents }: KpiStripProps) {
  const latencies = checks.filter((c) => c.up).map((c) => c.latency_ms)
  const min = latencies.length > 0 ? Math.min(...latencies) : null
  const max = latencies.length > 0 ? Math.max(...latencies) : null
  const avg =
    latencies.length > 0
      ? latencies.reduce((sum, v) => sum + v, 0) / latencies.length
      : null
  const p95 = percentile(latencies, 95)

  const resolved = incidents.filter((i) => i.duration_seconds > 0)
  const ongoingSeconds = incidents
    .filter((i) => i.duration_seconds === 0)
    .reduce((sum, i) => sum + Math.max(0, (Date.now() - new Date(i.start).getTime()) / 1000), 0)
  const downtimeSeconds =
    resolved.reduce((sum, i) => sum + i.duration_seconds, 0) + ongoingSeconds
  const mttrSeconds =
    resolved.length > 0
      ? resolved.reduce((sum, i) => sum + i.duration_seconds, 0) / resolved.length
      : null

  const lat = (v: number | null) => (v === null ? '—' : formatLatency(v))

  return (
    <div>
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
        <KpiTile label="Min" value={lat(min)} />
        <KpiTile label="Avg" value={lat(avg)} />
        <KpiTile label="Max" value={lat(max)} />
        <KpiTile label="P95" value={lat(p95)} />
        <KpiTile label="MTTR" value={mttrSeconds === null ? '—' : formatDuration(mttrSeconds)} />
        <KpiTile
          label="Downtime"
          value={incidents.length === 0 ? '—' : formatDuration(downtimeSeconds)}
        />
      </div>
      <p className="mt-2 text-[11px] text-zinc-600">
        Latency from the visible window · MTTR and downtime from the 5 most recent incidents
      </p>
    </div>
  )
}
