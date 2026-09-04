import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'

export function Header({ right }: { right?: ReactNode }) {
  return (
    <header className="sticky top-0 z-40 border-b border-zinc-800/60 bg-zinc-950/75 backdrop-blur">
      <div className="mx-auto flex h-14 max-w-5xl items-center justify-between gap-4 px-4">
        <div className="flex items-center gap-5">
          <Link
            to="/"
            className="flex items-center gap-2 rounded-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-400/40"
          >
            <span className="h-2.5 w-2.5 rounded-full bg-emerald-500" />
            <span className="text-sm font-semibold tracking-tight text-zinc-100">noroshi</span>
          </Link>
          <nav className="flex items-center gap-1 text-sm" aria-label="Main navigation">
            <NavLink to="/" exact>
              Endpoints
            </NavLink>
            <NavLink to="/maintenance">Maintenance</NavLink>
          </nav>
        </div>
        {right}
      </div>
    </header>
  )
}

function NavLink({
  to,
  exact = false,
  children,
}: {
  to: string
  exact?: boolean
  children: ReactNode
}) {
  return (
    <Link
      to={to}
      activeOptions={{ exact }}
      className="rounded-md px-2 py-1 text-zinc-400 transition-colors duration-150 hover:text-zinc-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-400/40"
      activeProps={{ className: 'text-zinc-100 bg-zinc-800/60' }}
    >
      {children}
    </Link>
  )
}
