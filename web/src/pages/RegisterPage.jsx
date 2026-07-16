import { useState } from 'react'
import { Link, Navigate, useNavigate } from 'react-router-dom'
import { api } from '../lib/api'
import { useAuthStore } from '../store/authStore'

const USERNAME_RE = /^[a-zA-Z0-9_]{2,32}$/

export default function RegisterPage() {
  const navigate = useNavigate()
  const user = useAuthStore((s) => s.user)
  const setSession = useAuthStore((s) => s.setSession)
  const [username, setUsername] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  if (user) return <Navigate to="/" replace />

  const submit = async (e) => {
    e.preventDefault()
    setError('')
    if (!USERNAME_RE.test(username.trim())) {
      setError('Username must be 2–32 characters: letters, numbers, underscores.')
      return
    }
    if (password.length < 8) {
      setError('Password must be at least 8 characters.')
      return
    }
    setBusy(true)
    try {
      const data = await api.postNoAuth('/auth/register', {
        username: username.trim(),
        email: email.trim().toLowerCase(),
        password,
      })
      setSession(data)
      navigate('/', { replace: true })
    } catch (err) {
      setError(err.message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="auth-page">
      <form className="auth-card" onSubmit={submit}>
        <h1>Create an account</h1>
        <p className="auth-sub">Join Discurd and start chatting.</p>
        {error && <div className="form-error">{error}</div>}
        <label className="field">
          <span>Username</span>
          <input autoFocus required autoComplete="username" maxLength={32} value={username} onChange={(e) => setUsername(e.target.value)} />
        </label>
        <label className="field">
          <span>Email</span>
          <input type="email" required autoComplete="email" value={email} onChange={(e) => setEmail(e.target.value)} />
        </label>
        <label className="field">
          <span>Password</span>
          <input
            type="password"
            required
            autoComplete="new-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </label>
        <button className="btn primary full" disabled={busy}>
          {busy ? 'Creating account…' : 'Register'}
        </button>
        <p className="auth-switch">
          Already have an account? <Link to="/login">Log In</Link>
        </p>
      </form>
    </div>
  )
}
