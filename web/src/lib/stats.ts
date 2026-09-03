import type { DayStat } from '../api/types'

// Percentile of an unsorted list; null when empty.
export function percentile(values: number[], p: number): number | null {
  if (values.length === 0) return null
  const sorted = [...values].sort((a, b) => a - b)
  const idx = Math.max(0, Math.min(Math.ceil((p / 100) * sorted.length) - 1, sorted.length - 1))
  return sorted[idx]
}

// Weighted overall uptime across daily stats; null when there are no checks.
export function overallUptime(days: DayStat[] | undefined): number | null {
  if (!days) return null
  const total = days.reduce((sum, d) => sum + d.total, 0)
  if (total === 0) return null
  const up = days.reduce((sum, d) => sum + d.up, 0)
  return (up / total) * 100
}
