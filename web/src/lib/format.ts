export function formatDuration(seconds: number): string {
  if (seconds < 60) return `${Math.round(seconds)}s`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) {
    const s = Math.round(seconds % 60)
    return s > 0 ? `${minutes}m ${s}s` : `${minutes}m`
  }
  const hours = Math.floor(minutes / 60)
  if (hours < 24) {
    const m = minutes % 60
    return m > 0 ? `${hours}h ${m}m` : `${hours}h`
  }
  const days = Math.floor(hours / 24)
  const h = hours % 24
  return h > 0 ? `${days}d ${h}h` : `${days}d`
}

export function relativeTime(iso: string | null): string {
  if (!iso) return 'never'
  const diffMs = Date.now() - new Date(iso).getTime()
  if (Number.isNaN(diffMs)) return 'never'
  const seconds = Math.floor(diffMs / 1000)
  if (seconds < 5) return 'just now'
  if (seconds < 60) return `${seconds}s ago`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  if (days < 30) return `${days}d ago`
  return new Date(iso).toLocaleDateString()
}

export function formatLatency(ms: number): string {
  if (ms >= 1000) {
    const s = ms / 1000
    return `${parseFloat(s.toFixed(s >= 10 ? 0 : 1))}s`
  }
  return `${Math.round(ms)}ms`
}

// Uptime percentages: values >= 99.99 get up to 4 decimals (trailing zeros
// trimmed) so high uptimes stay distinguishable; everything else 2 decimals.
export function formatUptime(uptime: number): string {
  if (uptime >= 99.99) return String(parseFloat(uptime.toFixed(4)))
  return uptime.toFixed(2)
}

export function shortTime(d: Date): string {
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

export function shortDate(d: Date): string {
  return d.toLocaleDateString([], { month: 'short', day: 'numeric' })
}

// Parse a Go-style duration ("10s", "5m", "1h30m", case/whitespace-lenient)
// into seconds. Plain numbers without a unit are rejected (returns null).
export function parseDuration(input: string): number | null {
  const s = input.trim().toLowerCase().replace(/\s+/g, '')
  if (!s) return null
  const re = /(\d+(?:\.\d+)?)([smh])/g
  let total = 0
  let consumed = ''
  let match: RegExpExecArray | null
  while ((match = re.exec(s)) !== null) {
    consumed += match[0]
    const value = parseFloat(match[1])
    total += match[2] === 'h' ? value * 3600 : match[2] === 'm' ? value * 60 : value
  }
  if (consumed !== s || total <= 0) return null
  return Math.round(total)
}

// Render an interval in seconds using the same duration convention as
// parseDuration: 45 -> "45s", 300 -> "5m", 90 -> "1m30s", 5400 -> "1h30m".
export function formatInterval(seconds: number): string {
  if (seconds < 60) return `${seconds}s`
  if (seconds < 3600) {
    const m = Math.floor(seconds / 60)
    const s = seconds % 60
    return s > 0 ? `${m}m${s}s` : `${m}m`
  }
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  return m > 0 ? `${h}h${m}m` : `${h}h`
}
