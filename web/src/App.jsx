import { useEffect, useState } from 'react'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { useAuthStore } from './store/authStore'
import { api, refreshSession } from './lib/api'
import LoginPage from './pages/LoginPage'
import RegisterPage from './pages/RegisterPage'
import AppShell from './pages/AppShell'
import Toasts from './components/Toasts'

function RequireAuth({ children }) {
  const user = useAuthStore((s) => s.user)
  const accessToken = useAuthStore((s) => s.accessToken)
  const refreshToken = useAuthStore((s) => s.refreshToken)
  const setUser = useAuthStore((s) => s.setUser)
  const [failed, setFailed] = useState(false)

  // Session bootstrap after a page reload: we only have the persisted refresh
  // token, so silently mint an access token and fetch the current user.
  useEffect(() => {
    if (user || !refreshToken) return undefined
    let cancelled = false
    ;(async () => {
      try {
        if (!useAuthStore.getState().accessToken) await refreshSession()
        const me = await api.get('/users/@me')
        if (!cancelled) setUser(me)
      } catch {
        if (!cancelled) setFailed(true)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [user, refreshToken, setUser])

  if ((!refreshToken && !accessToken) || failed) return <Navigate to="/login" replace />
  if (!user) return <div className="boot-screen">Loading Discurd…</div>
  return children
}

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/register" element={<RegisterPage />} />
        <Route
          path="/*"
          element={
            <RequireAuth>
              <AppShell />
            </RequireAuth>
          }
        />
      </Routes>
      <Toasts />
    </BrowserRouter>
  )
}
