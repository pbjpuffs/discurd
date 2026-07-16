import { useAuthStore } from '../store/authStore'

const BASE = '/api/v1'

export class ApiError extends Error {
  constructor(status, code, message) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

// ---- silent refresh (single-flight: concurrent 401s share one refresh) ----

let refreshInFlight = null

export function refreshSession() {
  if (!refreshInFlight) {
    refreshInFlight = doRefresh().finally(() => {
      refreshInFlight = null
    })
  }
  return refreshInFlight
}

async function doRefresh() {
  const { refreshToken, setSession, clearSession } = useAuthStore.getState()
  if (!refreshToken) throw new ApiError(401, 'unauthorized', 'Not signed in')
  const res = await fetch(`${BASE}/auth/refresh`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ refresh_token: refreshToken }),
  })
  if (!res.ok) {
    // Only a definitive auth rejection means the refresh token is actually
    // dead. A 429 (rate limit) or 5xx (transient backend blip) must NOT log the
    // user out — throw a retryable error and keep the session so the caller /
    // WS backoff can try again. (Network errors already throw before here.)
    if (res.status === 401 || res.status === 403) {
      clearSession()
      throw new ApiError(res.status, 'unauthorized', 'Session expired, please log in again')
    }
    throw new ApiError(res.status, 'refresh_unavailable', 'Could not refresh session, retrying')
  }
  const data = await res.json()
  setSession({ access_token: data.access_token, refresh_token: data.refresh_token })
  return data.access_token
}

function tokenExpiringSoon(token) {
  try {
    const payload = JSON.parse(atob(token.split('.')[1].replace(/-/g, '+').replace(/_/g, '/')))
    if (!payload.exp) return false
    return payload.exp * 1000 - Date.now() < 15000
  } catch {
    return false
  }
}

// Returns a usable access token, refreshing first if the current one is
// missing or about to expire (used by the WebSocket client before identify).
export async function ensureFreshAccessToken() {
  const { accessToken, refreshToken } = useAuthStore.getState()
  if (accessToken && !tokenExpiringSoon(accessToken)) return accessToken
  if (!refreshToken) throw new ApiError(401, 'unauthorized', 'Not signed in')
  return refreshSession()
}

// ---- request core ----

async function request(path, { method = 'GET', body, formData, auth = true, retried = false } = {}) {
  const headers = {}
  if (auth) {
    const token = useAuthStore.getState().accessToken
    if (token) headers.Authorization = `Bearer ${token}`
  }
  let reqBody
  if (formData) {
    reqBody = formData // browser sets multipart boundary
  } else if (body !== undefined) {
    headers['Content-Type'] = 'application/json'
    reqBody = JSON.stringify(body)
  }

  const res = await fetch(`${BASE}${path}`, { method, headers, body: reqBody })

  if (res.status === 401 && auth && !retried) {
    await refreshSession() // throws (and clears the session) if it fails
    return request(path, { method, body, formData, auth, retried: true })
  }

  if (res.status === 204) return null

  let data = null
  try {
    data = await res.json()
  } catch {
    // no JSON body
  }
  if (!res.ok) {
    const err = (data && data.error) || {}
    throw new ApiError(res.status, err.code || 'internal', err.message || `Request failed (${res.status})`)
  }
  return data
}

export const api = {
  get: (path) => request(path),
  post: (path, body) => request(path, { method: 'POST', body }),
  patch: (path, body) => request(path, { method: 'PATCH', body }),
  del: (path) => request(path, { method: 'DELETE' }),
  upload: (path, file) => {
    const fd = new FormData()
    fd.append('file', file)
    return request(path, { method: 'POST', formData: fd })
  },
  postNoAuth: (path, body) => request(path, { method: 'POST', body, auth: false }),
}
