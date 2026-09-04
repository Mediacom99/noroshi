import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createRouter, RouterProvider } from '@tanstack/react-router'
import { rootRoute } from './routes/root'
import { loginRoute } from './routes/login'
import { indexRoute } from './routes'
import { endpointDetailRoute } from './routes/endpoints.$id'
import { maintenanceRoute } from './routes/maintenance'
import './index.css'

const routeTree = rootRoute.addChildren([
  loginRoute,
  indexRoute,
  endpointDetailRoute,
  maintenanceRoute,
])

const router = createRouter({ routeTree })

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}

const queryClient = new QueryClient()

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </StrictMode>,
)
