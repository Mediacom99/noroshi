import type { Check } from '../api/types'
import { shortTime } from '../lib/format'
import { statusTokens } from '../lib/status'

interface UptimeBarProps {
  checks: Check[] | undefined
  paused?: boolean
  segments?: number
  windowMs?: number
  barHeight?: string
}

const DAY_MS = 24 * 60 * 60 * 1000

// Classic status-page uptime bar: one segment per time bucket.
// Emerald if all checks in the bucket are up, rose if any is down,
// hollow zinc for empty buckets, solid zinc when paused.
export function UptimeBar({
  checks,
  paused = false,
  segments = 48,
  windowMs = DAY_MS,
  barHeight = 'h-6',
}: UptimeBarProps) {
  const end = Date.now()
  const start = end - windowMs
  const segMs = windowMs / segments

  const buckets = Array.from({ length: segments }, () => ({ total: 0, down: 0 }))
  if (!paused && checks) {
    for (const check of checks) {
      const t = new Date(check.checked_at).getTime()
      if (Number.isNaN(t) || t < start || t > end) continue
      const idx = Math.min(Math.floor((t - start) / segMs), segments - 1)
      buckets[idx].total++
      if (!check.up) buckets[idx].down++
    }
  }

  return (
    <div className="flex gap-0.5" role="img" aria-label="Uptime over the last 24 hours">
      {buckets.map((bucket, i) => {
        const segStart = new Date(start + i * segMs)
        const segEnd = new Date(segStart.getTime() + segMs)
        const color = paused
          ? statusTokens.paused.bar
          : bucket.total === 0
            ? 'bg-zinc-800/70'
            : bucket.down > 0
              ? statusTokens.down.bar
              : statusTokens.up.bar
        const summary = paused
          ? 'paused'
          : bucket.total === 0
            ? 'no data'
            : bucket.down === 0
              ? `${bucket.total} check${bucket.total === 1 ? '' : 's'}, all up`
              : `${bucket.down}/${bucket.total} checks down`
        const align =
          i < 2
            ? 'left-0'
            : i > segments - 3
              ? 'right-0'
              : 'left-1/2 -translate-x-1/2'
        return (
          <div key={i} className="group/seg relative flex-1">
            <div
              className={`${barHeight} w-full rounded-[3px] ${color} transition-opacity duration-150 group-hover/seg:opacity-75`}
            />
            <div
              className={`pointer-events-none absolute bottom-full z-30 mb-2 hidden rounded-md border border-zinc-800 bg-zinc-900 px-2 py-1 whitespace-nowrap shadow-lg group-hover/seg:block ${align}`}
            >
              <span className="text-[11px] tabular-nums text-zinc-400">
                {shortTime(segStart)}–{shortTime(segEnd)}
              </span>
              <span className="ml-1.5 text-[11px] text-zinc-300">{summary}</span>
            </div>
          </div>
        )
      })}
    </div>
  )
}
