import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { useProfileStore } from '../store/profileStore'
import { useChatStore } from '../store/chatStore'
import { useAuthStore } from '../store/authStore'
import Avatar from './Avatar'
import EditProfileModal from './EditProfileModal'

const CARD_W = 320
const GAP = 8

const HEX_RE = /^#[0-9a-fA-F]{6}$/

function formatMemberSince(iso) {
  if (!iso) return null
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return null
  return d.toLocaleDateString([], { year: 'numeric', month: 'long', day: 'numeric' })
}

export default function ProfileCard() {
  const open = useProfileStore((s) => s.open)
  const closeCard = useProfileStore((s) => s.closeCard)
  const profile = useProfileStore((s) => (s.open ? s.profiles[s.open.userId] : null))
  const loading = useProfileStore((s) => (s.open ? s.loading[s.open.userId] : false))
  const errored = useProfileStore((s) => (s.open ? s.errored[s.open.userId] : false))
  const me = useAuthStore((s) => s.user)

  // Presence: scan loaded member lists for this user; treat "me" as online.
  const status = useChatStore((s) => {
    if (!s.selectedGuildId || !open) return null
    for (const gid of Object.keys(s.membersByGuild)) {
      const m = (s.membersByGuild[gid] || []).find((x) => x.user_id === open.userId)
      if (m) return m.status || 'offline'
    }
    return null
  })

  const cardRef = useRef(null)
  const [pos, setPos] = useState({ left: 0, top: 0 })
  const [editing, setEditing] = useState(false)

  // Close on outside click / Escape.
  useEffect(() => {
    if (!open) return undefined
    const onDown = (e) => {
      if (cardRef.current && !cardRef.current.contains(e.target)) closeCard()
    }
    const onKey = (e) => {
      if (e.key === 'Escape') closeCard()
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open, closeCard])

  // Reset the edit sub-modal whenever a different card opens.
  useEffect(() => {
    setEditing(false)
  }, [open && open.userId])

  // Position the card next to its anchor, clamped to the viewport.
  useLayoutEffect(() => {
    if (!open || !cardRef.current) return
    const rect = open.rect
    const cardH = cardRef.current.offsetHeight || 320
    const vw = window.innerWidth
    const vh = window.innerHeight
    let left = rect.right + GAP
    if (left + CARD_W > vw - 8) left = rect.left - GAP - CARD_W
    if (left < 8) left = Math.min(rect.left, vw - CARD_W - 8)
    left = Math.max(8, left)
    let top = rect.top
    if (top + cardH > vh - 8) top = vh - cardH - 8
    top = Math.max(8, top)
    setPos({ left, top })
  }, [open, profile, loading])

  if (!open) return null

  const isMe = !!(me && open.userId === me.id)
  const p = profile || (isMe ? me : null)
  const accent = p && HEX_RE.test(p.accent_color || '') ? p.accent_color : '#5865f2'
  const online = isMe || status === 'online'
  const memberSince = p ? formatMemberSince(p.created_at) : null

  return (
    <>
      <div className="profile-card" ref={cardRef} style={{ left: pos.left, top: pos.top, width: CARD_W }}>
        <div className="profile-banner" style={{ background: accent }} />
        <div className="profile-avatar-wrap">
          <Avatar name={(p && p.username) || '?'} url={p && p.avatar_url} size={80} />
          <span className={`presence-dot big ${online ? 'online' : 'off'}`} />
        </div>
        <div className="profile-body">
          {loading && !p && <div className="profile-loading">Loading…</div>}
          {errored && !p && <div className="profile-loading">Couldn’t load this profile.</div>}
          {p && (
            <>
              <div className="profile-name-row">
                <span className="profile-username">{p.username}</span>
                {p.pronouns ? <span className="profile-pronouns">{p.pronouns}</span> : null}
              </div>
              <div className="profile-status-line">
                <span className={`profile-status-dot ${online ? 'online' : 'off'}`} />
                {online ? 'Online' : 'Offline'}
              </div>
              {memberSince && (
                <div className="profile-section">
                  <div className="profile-section-label">Member Since</div>
                  <div className="profile-section-text">{memberSince}</div>
                </div>
              )}
              {p.bio ? (
                <div className="profile-section">
                  <div className="profile-section-label">About Me</div>
                  <div className="profile-section-text bio">{p.bio}</div>
                </div>
              ) : null}
              {isMe && (
                <button className="btn small full profile-edit-btn" onClick={() => setEditing(true)}>
                  Edit Profile
                </button>
              )}
            </>
          )}
        </div>
      </div>
      {editing && isMe && <EditProfileModal onClose={() => setEditing(false)} />}
    </>
  )
}
