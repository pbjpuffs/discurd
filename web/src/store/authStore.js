import { create } from 'zustand'

const REFRESH_KEY = 'discurd_refresh_token'

// Access token lives in memory only; the refresh token is persisted so the
// session survives reloads (silently re-established via /auth/refresh).
export const useAuthStore = create((set) => ({
  user: null,
  accessToken: null,
  refreshToken: localStorage.getItem(REFRESH_KEY),

  setSession: ({ user, access_token, refresh_token }) => {
    if (refresh_token) localStorage.setItem(REFRESH_KEY, refresh_token)
    set((s) => ({
      user: user !== undefined ? user : s.user,
      accessToken: access_token,
      refreshToken: refresh_token !== undefined ? refresh_token : s.refreshToken,
    }))
  },

  setUser: (user) => set({ user }),

  clearSession: () => {
    localStorage.removeItem(REFRESH_KEY)
    set({ user: null, accessToken: null, refreshToken: null })
  },
}))

// Keep the in-memory refresh token in sync across tabs. Refresh tokens are
// single-use and rotated on every refresh, so when another tab rotates it this
// tab must adopt the new value or its next refresh would fail with the (now
// invalid) stale token. We only adopt a newer token; we don't cascade clears.
if (typeof window !== 'undefined') {
  window.addEventListener('storage', (e) => {
    if (e.key === REFRESH_KEY && e.newValue) {
      useAuthStore.setState({ refreshToken: e.newValue })
    }
  })
}
