export function TypeChip({ type }: { type: string }) {
  return (
    <span className="rounded border border-zinc-700/80 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-zinc-400">
      {type}
    </span>
  )
}
