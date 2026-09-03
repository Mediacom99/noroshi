// Semantic status tokens — the single source of truth for status colors.
// up = emerald, down = rose, unknown = amber, paused = zinc.

export type StatusKind = 'up' | 'down' | 'unknown' | 'paused'

export function statusKind(status: string, paused: boolean): StatusKind {
  if (paused) return 'paused'
  if (status === 'ok') return 'up'
  if (status === 'not_ok') return 'down'
  return 'unknown'
}

export interface StatusToken {
  label: string
  dot: string
  text: string
  bar: string
  badge: string
}

export const statusTokens: Record<StatusKind, StatusToken> = {
  up: {
    label: 'Operational',
    dot: 'bg-emerald-500',
    text: 'text-emerald-400',
    bar: 'bg-emerald-500',
    badge: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-400',
  },
  down: {
    label: 'Down',
    dot: 'bg-rose-500',
    text: 'text-rose-400',
    bar: 'bg-rose-500',
    badge: 'border-rose-500/30 bg-rose-500/10 text-rose-400',
  },
  unknown: {
    label: 'Unknown',
    dot: 'bg-amber-500',
    text: 'text-amber-400',
    bar: 'bg-amber-500',
    badge: 'border-amber-500/30 bg-amber-500/10 text-amber-400',
  },
  paused: {
    label: 'Paused',
    dot: 'bg-zinc-500',
    text: 'text-zinc-400',
    bar: 'bg-zinc-600/60',
    badge: 'border-zinc-500/30 bg-zinc-500/10 text-zinc-400',
  },
}

export function uptimeTextColor(uptime: number): string {
  if (uptime >= 99.9) return 'text-emerald-400'
  if (uptime >= 99) return 'text-amber-400'
  return 'text-rose-400'
}

export function uptimeBarColor(uptime: number): string {
  if (uptime >= 99.9) return 'bg-emerald-500'
  if (uptime >= 99) return 'bg-amber-500'
  return 'bg-rose-500'
}

export function uptimeBadgeColor(uptime: number): string {
  if (uptime >= 99.9) return 'border-emerald-500/30 bg-emerald-500/10 text-emerald-400'
  if (uptime >= 99) return 'border-amber-500/30 bg-amber-500/10 text-amber-400'
  return 'border-rose-500/30 bg-rose-500/10 text-rose-400'
}
