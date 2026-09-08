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

// Parse a duration into seconds. Units: s (seconds), m (minutes), h (hours),
// d (days), w (weeks) — composable Go-style ("1h30m", "1w2d"), case/whitespace
// lenient. A bare number is treated as seconds. Invalid input returns null.
export function parseDuration(input: string): number | null {
  const s = input.trim().toLowerCase().replace(/\s+/g, '')
  if (!s) return null
  if (/^\d+$/.test(s)) {
    const n = parseInt(s, 10)
    return n > 0 ? n : null
  }
  const factors: Record<string, number> = { s: 1, m: 60, h: 3600, d: 86400, w: 604800 }
  const re = /(\d+(?:\.\d+)?)([smhdw])/g
  let total = 0
  let consumed = ''
  let match: RegExpExecArray | null
  while ((match = re.exec(s)) !== null) {
    consumed += match[0]
    total += parseFloat(match[1]) * factors[match[2]]
  }
  if (consumed !== s || total <= 0) return null
  return Math.round(total)
}

// Render an interval in seconds using the same duration convention as
// parseDuration: 45 -> "45s", 300 -> "5m", 5400 -> "1h30m", 90000 -> "1d1h",
// 1209600 -> "2w".
export function formatInterval(seconds: number): string {
  const parts: string[] = []
  let rest = seconds
  for (const [unit, size] of [
    ['w', 604800],
    ['d', 86400],
    ['h', 3600],
    ['m', 60],
  ] as const) {
    if (rest >= size) {
      parts.push(`${Math.floor(rest / size)}${unit}`)
      rest %= size
    }
  }
  if (rest > 0 || parts.length === 0) parts.push(`${rest}s`)
  return parts.join('')
}
