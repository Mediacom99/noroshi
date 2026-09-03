import { createRootRoute, redirect, Outlet } from '@tanstack/react-router'
import { getToken } from '../api/client'

export const rootRoute = createRootRoute({
  beforeLoad: ({ location }) => {
    if (!getToken() && location.pathname !== '/login') {
      throw redirect({ to: '/login' })
    }
  },
  component: Outlet,
})
