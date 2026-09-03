import type { DayStat } from '../api/types'
import { formatUptime, shortDate } from '../lib/format'
import { uptimeBarColor } from '../lib/status'

interface DailyUptimeStripProps {
  days: DayStat[] | undefined
  count?: number
  barHeight?: string
}

// Better Stack / statuspage-style daily bars, oldest -> newest, ending today
// (UTC). Days missing from the API response render as hollow zinc bars.
export function DailyUptimeStrip({ days, count = 30, barHeight = 'h-8' }: DailyUptimeStripProps) {
  const byDate = new Map((days ?? []).map((d) => [d.date, d]))
  const now = new Date()
  const todayUtc = Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate())

  const slots = Array.from({ length: count }, (_, i) => {
    const date = new Date(todayUtc - (count - 1 - i) * 24 * 60 * 60 * 1000).toISOString().slice(0, 10)
    return { date, stat: byDate.get(date) }
  })

  return (
    <div className="flex gap-[3px]" role="img" aria-label={`Daily uptime for the last ${count} days`}>
      {slots.map(({ date, stat }, i) => {
        const color = !stat || stat.total === 0 ? 'bg-zinc-800/70' : uptimeBarColor(stat.uptime)
        const align =
          i < 2 ? 'left-0' : i > count - 3 ? 'right-0' : 'left-1/2 -translate-x-1/2'
        return (
          <div key={date} className="group/seg relative flex-1">
            <div
              className={`${barHeight} w-full rounded-[2px] ${color} transition-opacity duration-150 group-hover/seg:opacity-75`}
            />
            <div
              className={`pointer-events-none absolute bottom-full z-30 mb-2 hidden rounded-md border border-zinc-800 bg-zinc-900 px-2 py-1 whitespace-nowrap shadow-lg group-hover/seg:block ${align}`}
            >
              <span className="text-[11px] text-zinc-400">
                {shortDate(new Date(`${date}T00:00:00Z`))}
              </span>
              <span className="ml-1.5 text-[11px] tabular-nums text-zinc-300">
                {!stat || stat.total === 0
                  ? 'no data'
                  : `${formatUptime(stat.uptime)}% · ${stat.up}/${stat.total} up`}
              </span>
            </div>
          </div>
        )
      })}
    </div>
  )
}
