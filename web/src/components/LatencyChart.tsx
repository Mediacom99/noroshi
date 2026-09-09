import { useMemo, useRef, useState } from 'react'
import type { MouseEvent } from 'react'
import type { Check, StatsWindow } from '../api/types'
import { formatLatency, shortDate, shortTime } from '../lib/format'
import { percentile } from '../lib/stats'

interface LatencyChartProps {
  checks: Check[]
  window: StatsWindow
}

const WINDOW_MS: Record<StatsWindow, number> = {
  '1h': 60 * 60 * 1000,
  '24h': 24 * 60 * 60 * 1000,
  '7d': 7 * 24 * 60 * 60 * 1000,
  '30d': 30 * 24 * 60 * 60 * 1000,
}

const GRID_FRACTIONS = [1, 2 / 3, 1 / 3]
const TOP_PCT = 8
const BOTTOM_PCT = 88

// Hand-rolled SVG area chart. Gridlines, dots and the crosshair are HTML
// overlays positioned in percent so they stay crisp while the SVG
// (viewBox 0..100, non-scaling stroke) stretches to fill the container.
// X positions are time-based: a check plots where it actually happened in
// the window, so freshly added endpoints don't fake a full-window history.
export function LatencyChart({ checks, window }: LatencyChartProps) {
  const sorted = useMemo(
    () => [...checks].sort((a, b) => a.checked_at.localeCompare(b.checked_at)),
    [checks],
  )
  const [hover, setHover] = useState<number | null>(null)
  const containerRef = useRef<HTMLDivElement>(null)

  if (sorted.length === 0) {
    return (
      <div className="card flex h-44 items-center justify-center text-sm text-zinc-500">
        No check data for this window
      </div>
    )
  }

  const n = sorted.length
  const maxLatency = Math.max(...sorted.map((c) => c.latency_ms), 1)
  const upLatencies = sorted.filter((c) => c.up).map((c) => c.latency_ms)
  const avgLatency =
    upLatencies.length > 0
      ? upLatencies.reduce((sum, v) => sum + v, 0) / upLatencies.length
      : null
  const p95Latency = percentile(upLatencies, 95)
  const referenceLines: { ms: number; label: string }[] = []
  if (avgLatency !== null) referenceLines.push({ ms: avgLatency, label: 'avg' })
  // Skip p95 when it would sit within ~6% of the scale of avg — the two
  // dashed lines would be indistinguishable anyway.
  if (
    p95Latency !== null &&
    (avgLatency === null || Math.abs(p95Latency - avgLatency) / maxLatency > 0.06)
  ) {
    referenceLines.push({ ms: p95Latency, label: 'p95' })
  }

  const axisEnd = Date.now()
  const axisStart = axisEnd - WINDOW_MS[window]
  // Time-based x positions (ascending, since `sorted` is chronological).
  const xs = sorted.map((c) => {
    const t = new Date(c.checked_at).getTime()
    if (Number.isNaN(t)) return 0
    return Math.max(0, Math.min(100, ((t - axisStart) / WINDOW_MS[window]) * 100))
  })
  const yPct = (ms: number) => TOP_PCT + (1 - ms / maxLatency) * (BOTTOM_PCT - TOP_PCT)

  const points = sorted.map((c, i) => [xs[i], yPct(c.latency_ms)] as const)
  const linePath = points
    .map((p, i) => `${i === 0 ? 'M' : 'L'}${p[0].toFixed(2)},${p[1].toFixed(2)}`)
    .join(' ')
  const areaBottom = Math.min(100, Math.max(...points.map((p) => p[0])))
  const areaLeft = Math.max(0, Math.min(...points.map((p) => p[0])))
  const areaPath = `${linePath} L${areaBottom.toFixed(2)},100 L${areaLeft.toFixed(2)},100 Z`

  function handleMove(e: MouseEvent<HTMLDivElement>) {
    const rect = containerRef.current?.getBoundingClientRect()
    if (!rect || rect.width === 0) return
    const ratio = ((e.clientX - rect.left) / rect.width) * 100
    // Nearest point by x (xs is sorted ascending); linear scan with early exit.
    let best = 0
    for (let i = 0; i < n; i++) {
      if (Math.abs(xs[i] - ratio) <= Math.abs(xs[best] - ratio)) {
        best = i
      } else if (xs[i] > ratio) {
        break
      }
    }
    setHover(best)
  }

  const axisLabels = [0, 1, 2, 3, 4].map((i) => new Date(axisStart + (WINDOW_MS[window] * i) / 4))
  const formatAxis = (d: Date) =>
    window === '1h' || window === '24h' ? shortTime(d) : shortDate(d)

  const hoverCheck = hover !== null ? sorted[hover] : null

  return (
    <div>
      <div
        ref={containerRef}
        onMouseMove={handleMove}
        onMouseLeave={() => setHover(null)}
        className="card relative h-44 overflow-hidden"
        role="img"
        aria-label="Latency chart"
      >
        {GRID_FRACTIONS.map((f) => (
          <div
            key={f}
            className="absolute inset-x-0 border-t border-zinc-800/60"
            style={{ top: `${yPct(maxLatency * f)}%` }}
          >
            {/* Grid values on the left; the topmost sits below its line to
                avoid being clipped by the container's overflow-hidden. */}
            <span
              className={`absolute left-2 text-[10px] tabular-nums text-zinc-600 ${
                f === 1 ? 'top-0.5' : '-top-1.5 -translate-y-full'
              }`}
            >
              {formatLatency(maxLatency * f)}
            </span>
          </div>
        ))}

        {referenceLines.map((line) => (
          <div
            key={line.label}
            className="absolute inset-x-0 border-t border-dashed border-zinc-600/70"
            style={{ top: `${yPct(line.ms)}%` }}
          />
        ))}

        <svg
          viewBox="0 0 100 100"
          preserveAspectRatio="none"
          className="absolute inset-0 h-full w-full"
        >
          <defs>
            <linearGradient id="latencyFill" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="rgb(16 185 129)" stopOpacity="0.15" />
              <stop offset="100%" stopColor="rgb(16 185 129)" stopOpacity="0" />
            </linearGradient>
          </defs>
          {n > 1 && <path d={areaPath} fill="url(#latencyFill)" />}
          {n > 1 && (
            <path
              d={linePath}
              fill="none"
              stroke="rgb(16 185 129)"
              strokeWidth="1.5"
              vectorEffect="non-scaling-stroke"
              strokeLinejoin="round"
              strokeLinecap="round"
            />
          )}
        </svg>

        {n === 1 && (
          <div
            className={`absolute h-2 w-2 -translate-x-1/2 -translate-y-1/2 rounded-full ${
              sorted[0].up ? 'bg-emerald-400' : 'bg-rose-500'
            }`}
            style={{ left: `${xs[0]}%`, top: `${yPct(sorted[0].latency_ms)}%` }}
          />
        )}

        {sorted.map(
          (check, i) =>
            !check.up && (
              <div
                key={`fail-${check.checked_at}-${i}`}
                className="absolute h-1.5 w-1.5 -translate-x-1/2 -translate-y-1/2 rounded-full bg-rose-500"
                style={{ left: `${xs[i]}%`, top: `${yPct(check.latency_ms)}%` }}
              />
            ),
        )}

        {hover !== null && hoverCheck && (
          <>
            <div
              className="absolute inset-y-0 w-px bg-zinc-600"
              style={{ left: `${xs[hover]}%` }}
            />
            <div
              className={`absolute h-2 w-2 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-zinc-950 ${
                hoverCheck.up ? 'bg-emerald-400' : 'bg-rose-500'
              }`}
              style={{ left: `${xs[hover]}%`, top: `${yPct(hoverCheck.latency_ms)}%` }}
            />
            <div
              className="pointer-events-none absolute top-2 z-10 rounded-md border border-zinc-800 bg-zinc-900 px-2.5 py-1.5 text-xs shadow-lg"
              style={{
                left: `${xs[hover]}%`,
                transform: xs[hover] > 60 ? 'translateX(calc(-100% - 10px))' : 'translateX(10px)',
              }}
            >
              <p className="whitespace-nowrap text-zinc-500">
                {new Date(hoverCheck.checked_at).toLocaleString()}
              </p>
              <p
                className={`whitespace-nowrap font-medium tabular-nums ${
                  hoverCheck.up ? 'text-emerald-400' : 'text-rose-400'
                }`}
              >
                {hoverCheck.up ? formatLatency(hoverCheck.latency_ms) : 'Down'}
                {hoverCheck.status_code > 0 && (
                  <span className="ml-1.5 text-zinc-500">HTTP {hoverCheck.status_code}</span>
                )}
              </p>
            </div>
          </>
        )}
      </div>

      {referenceLines.length > 0 && (
        <div className="mt-1.5 flex flex-wrap gap-4 text-[10px] text-zinc-500">
          {referenceLines.map((line) => (
            <span key={line.label} className="flex items-center gap-1.5">
              <span className="inline-block w-4 border-t border-dashed border-zinc-600/70" />
              {line.label}
              <span className="tabular-nums text-zinc-400">{formatLatency(line.ms)}</span>
            </span>
          ))}
        </div>
      )}

      <div className="mt-2 flex justify-between text-[11px] tabular-nums text-zinc-600">
        {axisLabels.map((d, i) => (
          <span key={i}>{formatAxis(d)}</span>
        ))}
      </div>
    </div>
  )
}
