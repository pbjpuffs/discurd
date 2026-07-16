import { useRef, useState } from 'react'
import { useAuthStore } from '../store/authStore'
import { useChatStore } from '../store/chatStore'
import { api, ensureFreshAccessToken } from '../lib/api'
import { disconnectGateway } from '../lib/gateway'
import { toast } from '../store/toastStore'
import Avatar from './Avatar'

const USERNAME_RE = /^[a-zA-Z0-9_]{2,32}$/

export default function SettingsPopover() {
  const user = useAuthStore((s) => s.user)
  const setUser = useAuthStore((s) => s.setUser)
  const clearSession = useAuthStore((s) => s.clearSession)
  const [username, setUsername] = useState(user.username)
  const [preview, setPreview] = useState(null)
  const [uploading, setUploading] = useState(false)
  const [saving, setSaving] = useState(false)
  const fileRef = useRef(null)

  const onAvatarPick = async (e) => {
    const file = e.target.files && e.target.files[0]
    e.target.value = ''
    if (!file) return
    const localUrl = URL.createObjectURL(file)
    setPreview(localUrl) // instant preview while the upload runs
    setUploading(true)
    try {
      const res = await api.upload('/users/@me/avatar', file)
      setUser({ ...useAuthStore.getState().user, avatar_url: res.avatar_url })
    } catch (err) {
      toast(err.message)
    } finally {
      setUploading(false)
      setPreview(null)
      URL.revokeObjectURL(localUrl)
    }
  }

  const usernameValid = USERNAME_RE.test(username.trim())
  const usernameChanged = username.trim() !== user.username

  const saveUsername = async () => {
    if (!usernameValid || !usernameChanged || saving) return
    setSaving(true)
    try {
      const updated = await api.patch('/users/@me', { username: username.trim() })
      setUser(updated)
      toast('Username updated', 'success')
    } catch (err) {
      toast(err.message)
    } finally {
      setSaving(false)
    }
  }

  const logout = async () => {
    try {
      await ensureFreshAccessToken()
      const rt = useAuthStore.getState().refreshToken
      if (rt) await api.post('/auth/logout', { refresh_token: rt })
    } catch {
      // best-effort server-side invalidation; always clear locally
    }
    disconnectGateway()
    useChatStore.getState().reset()
    clearSession()
  }

  return (
    <div className="settings-popover" onMouseDown={(e) => e.stopPropagation()}>
      <div className="settings-avatar-row">
        <Avatar name={user.username} url={preview || user.avatar_url} size={56} />
        <div>
          <div className="settings-username">{user.username}</div>
          <button className="btn small" onClick={() => fileRef.current && fileRef.current.click()} disabled={uploading}>
            {uploading ? 'Uploading…' : 'Change Avatar'}
          </button>
          <input ref={fileRef} type="file" accept="image/*" hidden onChange={onAvatarPick} />
        </div>
      </div>
      <div className="settings-divider" />
      <label className="field">
        <span>Username</span>
        <input
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          maxLength={32}
          onKeyDown={(e) => {
            if (e.key === 'Enter') saveUsername()
          }}
        />
      </label>
      {!usernameValid && username.trim() !== '' && (
        <div className="form-error small">2–32 characters: letters, numbers, underscores.</div>
      )}
      <button className="btn primary full" onClick={saveUsername} disabled={!usernameValid || !usernameChanged || saving}>
        {saving ? 'Saving…' : 'Save Username'}
      </button>
      <div className="settings-divider" />
      <button className="btn danger full" onClick={logout}>
        Log Out
      </button>
    </div>
  )
}
