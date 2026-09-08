import type { DayStat } from '../api/types'
import { formatUptime, shortDate } from '../lib/format'
import { statusTokens, uptimeBarColor } from '../lib/status'

interface DailyUptimeBarProps {
  days: DayStat[] | undefined
  paused?: boolean
  count?: number
  barHeight?: string
}

const DAY_MS = 24 * 60 * 60 * 1000

// One block per UTC day over the last `count` days (default 30), so the bar
// matches the "30d" uptime figure shown next to it. Days without checks are
// hollow zinc; solid zinc when paused.
export function DailyUptimeBar({
  days,
  paused = false,
  count = 30,
  barHeight = 'h-6',
}: DailyUptimeBarProps) {
  const now = new Date()
  const todayUtc = Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate())
  const byDate = new Map((days ?? []).map((d) => [d.date, d]))

  const slots = Array.from({ length: count }, (_, i) => {
    const dayStart = todayUtc - (count - 1 - i) * DAY_MS
    const date = new Date(dayStart).toISOString().slice(0, 10)
    return { dayStart, stat: byDate.get(date) }
  })

  return (
    <div className="flex gap-0.5" role="img" aria-label={`Uptime per day over the last ${count} days`}>
      {slots.map(({ dayStart, stat }, i) => {
        const color = paused
          ? statusTokens.paused.bar
          : !stat || stat.total === 0
            ? 'bg-zinc-800/70'
            : uptimeBarColor(stat.uptime)
        const summary = paused
          ? 'paused'
          : !stat || stat.total === 0
            ? 'no data'
            : `${formatUptime(stat.uptime)}% · ${stat.total} check${stat.total === 1 ? '' : 's'}`
        const align =
          i < 2 ? 'left-0' : i > count - 3 ? 'right-0' : 'left-1/2 -translate-x-1/2'
        return (
          <div key={i} className="group/seg relative flex-1">
            <div
              className={`${barHeight} w-full rounded-[3px] ${color} transition-opacity duration-150 group-hover/seg:opacity-75`}
            />
            <div
              className={`pointer-events-none absolute bottom-full z-30 mb-2 hidden rounded-md border border-zinc-800 bg-zinc-900 px-2 py-1 whitespace-nowrap shadow-lg group-hover/seg:block ${align}`}
            >
              <span className="text-[11px] tabular-nums text-zinc-400">
                {shortDate(new Date(dayStart))}
              </span>
              <span className="ml-1.5 text-[11px] text-zinc-300">{summary}</span>
            </div>
          </div>
        )
      })}
    </div>
  )
}
