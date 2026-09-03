import { statusKind, statusTokens } from '../lib/status'

interface StatusDotProps {
  status: string
  paused?: boolean
  showLabel?: boolean
}

export function StatusDot({ status, paused = false, showLabel = true }: StatusDotProps) {
  const token = statusTokens[statusKind(status, paused)]

  return (
    <span className="inline-flex items-center gap-2">
      <span className={`h-2.5 w-2.5 shrink-0 rounded-full ${token.dot}`} />
      {showLabel && <span className={`text-sm ${token.text}`}>{token.label}</span>}
    </span>
  )
}
