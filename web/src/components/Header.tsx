import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'

export function Header({ right }: { right?: ReactNode }) {
  return (
    <header className="sticky top-0 z-40 border-b border-zinc-800/60 bg-zinc-950/75 backdrop-blur">
      <div className="mx-auto flex h-14 max-w-5xl items-center justify-between px-4">
        <Link
          to="/"
          className="flex items-center gap-2 rounded-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-400/40"
        >
          <span className="h-2.5 w-2.5 rounded-full bg-emerald-500" />
          <span className="text-sm font-semibold tracking-tight text-zinc-100">noroshi</span>
        </Link>
        {right}
      </div>
    </header>
  )
}
