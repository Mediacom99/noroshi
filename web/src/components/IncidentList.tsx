import type { Incident } from '../api/types'
import { formatDuration, relativeTime } from '../lib/format'

interface IncidentListProps {
  incidents: Incident[]
  relaxed?: boolean
}

export function IncidentList({ incidents, relaxed = false }: IncidentListProps) {
  if (incidents.length === 0) {
    return (
      <div className="card p-6 text-center text-sm text-zinc-500">
        No incidents recorded. Everything has been quiet.
      </div>
    )
  }

  return (
    <ol
      className={`relative ml-1 border-l border-zinc-800/80 ${
        relaxed ? 'space-y-6 pl-7' : 'space-y-5 pl-6'
      }`}
    >
      {incidents.map((incident, i) => (
        <li key={`${incident.start}-${i}`} className="relative">
          <span
            className={`absolute rounded-full bg-rose-500 ring-4 ring-zinc-950 ${
              relaxed ? 'top-1.5 -left-[34px] h-3 w-3' : 'top-1.5 -left-[29px] h-2.5 w-2.5'
            }`}
          />
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
            <p className={`${relaxed ? 'text-base' : 'text-sm'} text-zinc-200`}>
              {new Date(incident.start).toLocaleString()}
            </p>
            {incident.duration_seconds === 0 ? (
              <span className="rounded-full border border-rose-500/30 bg-rose-500/10 px-2 py-0.5 text-[11px] font-medium text-rose-400">
                Ongoing
              </span>
            ) : (
              <span className={`${relaxed ? 'text-sm' : 'text-xs'} tabular-nums text-zinc-500`}>
                lasted {formatDuration(incident.duration_seconds)}
              </span>
            )}
          </div>
          <p className={`mt-0.5 ${relaxed ? 'text-sm' : 'text-xs'} text-zinc-500`}>
            {incident.status_code > 0 ? `HTTP ${incident.status_code}` : 'Connection error'}
            {' · '}
            {relativeTime(incident.start)}
          </p>
        </li>
      ))}
    </ol>
  )
}
