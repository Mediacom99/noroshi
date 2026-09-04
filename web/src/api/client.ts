const TOKEN_KEY = 'noroshi_token'

// Empty VITE_API_URL means "same origin" — combined with the Vite dev proxy
// for /api this needs no CORS during local development.
const BASE_URL: string = import.meta.env.VITE_API_URL ?? 'http://localhost:8080'

// Origin the API is served from; used to build absolute URLs for badge assets.
export function apiOrigin(): string {
  return BASE_URL === '' ? window.location.origin : BASE_URL
}

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY)
}

export function setToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token)
}

export function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY)
}

export class ApiError extends Error {
  status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

export async function api<T>(path: string, options: RequestInit = {}): Promise<T> {
  const token = getToken()
  const res = await fetch(`${BASE_URL}${path}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...options.headers,
    },
  })

  if (res.status === 401) {
    clearToken()
    if (!window.location.pathname.startsWith('/login')) {
      window.location.href = '/login'
    }
    throw new ApiError(401, 'Unauthorized')
  }

  if (res.status === 204) {
    return undefined as T
  }

  const data: unknown = await res.json().catch(() => null)
  if (!res.ok) {
    const message =
      data && typeof data === 'object' && 'error' in data && typeof data.error === 'string'
        ? data.error
        : `Request failed with status ${res.status}`
    throw new ApiError(res.status, message)
  }
  return data as T
}
