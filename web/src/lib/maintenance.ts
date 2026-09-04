import type { MaintenanceWindow } from '../api/types'

export const DAY_CODES = ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'] as const

const DAY_LABELS: Record<string, string> = {
  mon: 'Mon',
  tue: 'Tue',
  wed: 'Wed',
  thu: 'Thu',
  fri: 'Fri',
  sat: 'Sat',
  sun: 'Sun',
}

export function formatDays(days: string): string {
  if (days === 'all') return 'Every day'
  return days
    .split(',')
    .map((d) => DAY_LABELS[d.trim().toLowerCase()] ?? d)
    .join(', ')
}

export function minutesToTime(minutes: number): string {
  const h = Math.floor(minutes / 60)
  const m = minutes % 60
  return `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}`
}

// A window covers an endpoint when it targets it specifically or all endpoints
// (endpoint_id null); "active" is computed server-side in UTC.
export function isEndpointInMaintenance(
  windows: MaintenanceWindow[] | undefined,
  endpointId: number,
): boolean {
  return (windows ?? []).some(
    (w) => w.active && (w.endpoint_id === null || w.endpoint_id === endpointId),
  )
}
