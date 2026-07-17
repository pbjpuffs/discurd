import { useRef, useState } from 'react'
import { useAuthStore } from '../store/authStore'
import { useProfileStore } from '../store/profileStore'
import { api } from '../lib/api'
import { toast } from '../store/toastStore'
import Modal from './Modal'
import Avatar from './Avatar'

const BIO_MAX = 190
const PRONOUNS_MAX = 40
const HEX_RE = /^#[0-9a-fA-F]{6}$/

export default function EditProfileModal({ onClose }) {
  const user = useAuthStore((s) => s.user)
  const setUser = useAuthStore((s) => s.setUser)
  const setProfile = useProfileStore((s) => s.setProfile)

  const [bio, setBio] = useState(user.bio || '')
  const [pronouns, setPronouns] = useState(user.pronouns || '')
  const [accent, setAccent] = useState(HEX_RE.test(user.accent_color || '') ? user.accent_color : '#5865f2')
  const [accentOn, setAccentOn] = useState(!!(user.accent_color && HEX_RE.test(user.accent_color)))
  const [preview, setPreview] = useState(null)
  const [uploading, setUploading] = useState(false)
  const [saving, setSaving] = useState(false)
  const fileRef = useRef(null)

  const onAvatarPick = async (e) => {
    const file = e.target.files && e.target.files[0]
    e.target.value = ''
    if (!file) return
    const localUrl = URL.createObjectURL(file)
    setPreview(localUrl)
    setUploading(true)
    try {
      const res = await api.upload('/users/@me/avatar', file)
      const updated = { ...useAuthStore.getState().user, avatar_url: res.avatar_url }
      setUser(updated)
      setProfile(updated)
    } catch (err) {
      toast(err.message)
    } finally {
      setUploading(false)
      setPreview(null)
      URL.revokeObjectURL(localUrl)
    }
  }

  const bioOver = bio.length > BIO_MAX
  const pronounsOver = pronouns.length > PRONOUNS_MAX
  const canSave = !saving && !uploading && !bioOver && !pronounsOver

  const save = async () => {
    if (!canSave) return
    setSaving(true)
    try {
      const body = {
        bio,
        pronouns,
        accent_color: accentOn ? accent : '',
      }
      const updated = await api.patch('/users/@me', body)
      setUser(updated)
      setProfile(updated)
      toast('Profile updated', 'success')
      onClose()
    } catch (err) {
      toast(err.message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <Modal title="Edit Profile" onClose={onClose}>
      <div className="settings-avatar-row">
        <Avatar name={user.username} url={preview || user.avatar_url} size={72} />
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
        <span>Pronouns</span>
        <input
          value={pronouns}
          maxLength={PRONOUNS_MAX + 10}
          placeholder="e.g. they/them"
          onChange={(e) => setPronouns(e.target.value)}
        />
      </label>
      {pronounsOver && <div className="form-error small">Pronouns must be {PRONOUNS_MAX} characters or fewer.</div>}

      <label className="field">
        <span>
          About Me
          <span className={`field-counter ${bioOver ? 'over' : ''}`}>{BIO_MAX - bio.length}</span>
        </span>
        <textarea
          className="profile-bio-input"
          value={bio}
          rows={3}
          maxLength={BIO_MAX + 40}
          placeholder="Tell everyone a little about yourself"
          onChange={(e) => setBio(e.target.value)}
        />
      </label>
      {bioOver && <div className="form-error small">Bio must be {BIO_MAX} characters or fewer.</div>}

      <div className="field">
        <span>Accent Color</span>
        <div className="accent-row">
          <label className="accent-toggle">
            <input type="checkbox" checked={accentOn} onChange={(e) => setAccentOn(e.target.checked)} />
            Use a custom accent
          </label>
          <input
            type="color"
            className="accent-color-input"
            value={accent}
            disabled={!accentOn}
            onChange={(e) => setAccent(e.target.value)}
          />
        </div>
      </div>

      <div className="modal-actions">
        <button className="btn ghost" onClick={onClose} disabled={saving}>
          Cancel
        </button>
        <button className="btn primary" onClick={save} disabled={!canSave}>
          {saving ? 'Saving…' : 'Save'}
        </button>
      </div>
    </Modal>
  )
}
