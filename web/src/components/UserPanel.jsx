import { useEffect, useRef, useState } from 'react'
import { useAuthStore } from '../store/authStore'
import Avatar from './Avatar'
import SettingsPopover from './SettingsPopover'
import { GearIcon } from './Icons'

export default function UserPanel() {
  const user = useAuthStore((s) => s.user)
  const [open, setOpen] = useState(false)
  const wrapRef = useRef(null)

  useEffect(() => {
    if (!open) return
    const onDoc = (e) => {
      if (wrapRef.current && !wrapRef.current.contains(e.target)) setOpen(false)
    }
    document.addEventListener('mousedown', onDoc)
    return () => document.removeEventListener('mousedown', onDoc)
  }, [open])

  if (!user) return null

  return (
    <div className="user-panel" ref={wrapRef}>
      <div className="user-panel-avatar">
        <Avatar name={user.username} url={user.avatar_url} size={32} />
        <span className="presence-dot online" />
      </div>
      <div className="user-panel-name">
        <div className="username" title={user.username}>
          {user.username}
        </div>
        <div className="user-sub">online</div>
      </div>
      <button
        className={`icon-btn gear ${open ? 'active' : ''}`}
        title="User Settings"
        onClick={() => setOpen((v) => !v)}
      >
        <GearIcon />
      </button>
      {open && <SettingsPopover />}
    </div>
  )
}
