import { formatUptime } from '../lib/format'
import { uptimeBadgeColor } from '../lib/status'

interface UptimeBadgeProps {
  uptime: number
}

export function UptimeBadge({ uptime }: UptimeBadgeProps) {
  return (
    <span
      className={`inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium tabular-nums ${uptimeBadgeColor(uptime)}`}
    >
      {formatUptime(uptime)}%
    </span>
  )
}
