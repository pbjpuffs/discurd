import { useEffect, useMemo, useRef, useState } from 'react'
import { useAuthStore } from '../store/authStore'
import { useChatStore } from '../store/chatStore'
import { useProfileStore, openProfileFromEvent } from '../store/profileStore'
import { toast } from '../store/toastStore'
import Avatar from './Avatar'
import EmojiPicker from './EmojiPicker'
import { formatTimestamp, formatTime, formatBytes } from '../lib/format'
import { PencilIcon, TrashIcon, FileIcon, SmileIcon } from './Icons'

export default function MessageItem({ msg, grouped, guildOwnerId, channelId }) {
  const me = useAuthStore((s) => s.user)
  const editMessage = useChatStore((s) => s.editMessage)
  const deleteMessage = useChatStore((s) => s.deleteMessage)
  const dismissLocal = useChatStore((s) => s.dismissLocal)
  const toggleReaction = useChatStore((s) => s.toggleReaction)
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState('')
  const [busy, setBusy] = useState(false)
  const [reactOpen, setReactOpen] = useState(false)
  const taRef = useRef(null)
  const reactRef = useRef(null)

  const isOwn = !!(me && msg.author.id === me.id)
  const canEdit = isOwn && !msg.optimistic
  const canDelete = (isOwn || (me && guildOwnerId === me.id)) && !msg.optimistic
  const canReact = !msg.optimistic && !msg.failed

  useEffect(() => {
    if (editing && taRef.current) {
      const ta = taRef.current
      ta.focus()
      ta.setSelectionRange(ta.value.length, ta.value.length)
    }
  }, [editing])

  // Close the reaction emoji picker on outside click.
  useEffect(() => {
    if (!reactOpen) return undefined
    const onDown = (e) => {
      if (reactRef.current && !reactRef.current.contains(e.target)) setReactOpen(false)
    }
    document.addEventListener('mousedown', onDown)
    return () => document.removeEventListener('mousedown', onDown)
  }, [reactOpen])

  const startEdit = () => {
    setDraft(msg.content)
    setEditing(true)
  }

  const saveEdit = async () => {
    const content = draft.trim()
    if (!content || content === msg.content) {
      setEditing(false)
      return
    }
    if (content.length > 4000) return
    setBusy(true)
    try {
      await editMessage(channelId, msg.id, content)
      setEditing(false)
    } catch (e) {
      toast(e.message)
    } finally {
      setBusy(false)
    }
  }

  const onEditKey = (e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      saveEdit()
    } else if (e.key === 'Escape') {
      setEditing(false)
    }
  }

  const remove = async () => {
    if (!window.confirm('Delete this message?')) return
    try {
      await deleteMessage(channelId, msg.id)
    } catch (e) {
      toast(e.message)
    }
  }

  const onPickReaction = (native) => {
    setReactOpen(false)
    toggleReaction(channelId, msg.id, native)
  }

  const cls = ['message', grouped ? 'grouped' : 'first', msg.optimistic ? 'pending' : '', msg.failed ? 'failed' : '']
    .filter(Boolean)
    .join(' ')

  const reactions = msg.reactions || []

  return (
    <div className={cls}>
      <div className="message-gutter">
        {grouped ? (
          <span className="hover-time">{formatTime(msg.created_at)}</span>
        ) : (
          <span className="author-avatar clickable" onClick={(e) => openProfileFromEvent(e, msg.author.id)}>
            <Avatar name={msg.author.username} url={msg.author.avatar_url} size={40} />
          </span>
        )}
      </div>
      <div className="message-body">
        {!grouped && (
          <div className="message-meta">
            <span className="message-author clickable" onClick={(e) => openProfileFromEvent(e, msg.author.id)}>
              {msg.author.username}
            </span>
            <span className="message-time">{formatTimestamp(msg.created_at)}</span>
          </div>
        )}
        {editing ? (
          <div className="message-edit">
            <textarea
              ref={taRef}
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              onKeyDown={onEditKey}
              rows={2}
              maxLength={4000}
              disabled={busy}
            />
            <div className="edit-hint">
              escape to{' '}
              <button className="link" onClick={() => setEditing(false)}>
                cancel
              </button>{' '}
              · enter to{' '}
              <button className="link" onClick={saveEdit}>
                save
              </button>
            </div>
          </div>
        ) : (
          <>
            {msg.content && (
              <div className="message-content">
                {msg.content}
                {msg.edited_at && <span className="edited-tag">(edited)</span>}
              </div>
            )}
            {msg.attachments && msg.attachments.length > 0 && (
              <div className="message-attachments">
                {msg.attachments.map((a, i) => (
                  <AttachmentView key={i} att={a} pending={!!msg.optimistic} />
                ))}
              </div>
            )}
            {reactions.length > 0 && (
              <ReactionRow reactions={reactions} msg={msg} me={me} onToggle={(emoji) => toggleReaction(channelId, msg.id, emoji)} />
            )}
            {msg.failed && (
              <div className="message-failed-note">
                Failed to send ·{' '}
                <button className="link" onClick={() => dismissLocal(channelId, msg.id)}>
                  dismiss
                </button>
              </div>
            )}
          </>
        )}
      </div>
      {!editing && (canReact || canEdit || canDelete) && (
        <div className={`message-actions ${reactOpen ? 'pinned' : ''}`}>
          {canReact && (
            <div className="react-wrap" ref={reactRef}>
              <button className="action-btn" title="Add Reaction" onClick={() => setReactOpen((v) => !v)}>
                <SmileIcon size={16} />
              </button>
              {reactOpen && <EmojiPicker className="reaction-emoji-pop" onPick={onPickReaction} />}
            </div>
          )}
          {canEdit && (
            <button className="action-btn" title="Edit" onClick={startEdit}>
              <PencilIcon size={16} />
            </button>
          )}
          {canDelete && (
            <button className="action-btn danger" title="Delete" onClick={remove}>
              <TrashIcon size={16} />
            </button>
          )}
        </div>
      )}
    </div>
  )
}

function ReactionRow({ reactions, msg, me, onToggle }) {
  // Resolve reactor user_ids (learned from live events) to display names via the
  // guild member list + profile cache; fall back to the raw count.
  const members = useChatStore((s) => (msg.guild_id ? s.membersByGuild[msg.guild_id] : null))
  const profiles = useProfileStore((s) => s.profiles)

  const nameFor = useMemo(() => {
    const map = new Map()
    for (const m of members || []) map.set(m.user_id, m.username)
    return (uid) => {
      if (me && uid === me.id) return 'You'
      if (map.has(uid)) return map.get(uid)
      const p = profiles[uid]
      return p ? p.username : null
    }
  }, [members, profiles, me])

  return (
    <div className="reactions-row">
      {reactions.map((r) => (
        <button
          key={r.emoji}
          className={`reaction-pill ${r.me ? 'reacted' : ''}`}
          onClick={() => onToggle(r.emoji)}
        >
          <span className="reaction-emoji">{r.emoji}</span>
          <span className="reaction-count">{r.count}</span>
          <span className="reaction-tooltip">{reactorsLabel(r, nameFor)}</span>
        </button>
      ))}
    </div>
  )
}

function reactorsLabel(reaction, nameFor) {
  const ids = reaction.users || []
  const names = ids.map((uid) => nameFor(uid)).filter(Boolean)
  const emoji = reaction.emoji
  if (names.length === 0) {
    const n = reaction.count
    return `${n} ${n === 1 ? 'reaction' : 'reactions'}`
  }
  const unknown = Math.max(0, reaction.count - names.length)
  let who
  if (names.length === 1) who = names[0]
  else if (names.length === 2) who = `${names[0]} and ${names[1]}`
  else who = `${names.slice(0, -1).join(', ')} and ${names[names.length - 1]}`
  if (unknown > 0) who += ` and ${unknown} other${unknown === 1 ? '' : 's'}`
  return `${who} reacted with ${emoji}`
}

// Own uploaded objects (/files/) are always allowed. Externally-hosted GIFs are
// allowed ONLY from Tenor/Giphy over https and ONLY when the attachment is an
// image/gif (matches the backend's attachment URL validation). Never emit an
// attacker-controllable scheme into href/src.
const GIF_HOST_RE = /(^|\.)(tenor\.com|giphy\.com)$/i

function safeAttachmentURL(att) {
  const url = att && att.url
  if (typeof url !== 'string') return null
  if (url.startsWith('/files/') && !url.includes('..')) return url
  if (att.content_type === 'image/gif') {
    try {
      const u = new URL(url)
      if (u.protocol === 'https:' && GIF_HOST_RE.test(u.hostname)) return url
    } catch {
      return null
    }
  }
  return null
}

function AttachmentView({ att, pending }) {
  const url = safeAttachmentURL(att)
  const isImage = url && att.content_type && att.content_type.startsWith('image/')
  if (isImage) {
    return (
      <a className="attachment-image" href={url} target="_blank" rel="noreferrer">
        <img src={url} alt={att.filename} loading="lazy" />
      </a>
    )
  }
  const inner = (
    <>
      <span className="file-icon">
        <FileIcon size={28} />
      </span>
      <span className="file-meta">
        <span className="file-name">{att.filename}</span>
        <span className="file-size">{pending && !url ? 'Uploading…' : formatBytes(att.size)}</span>
      </span>
    </>
  )
  if (!url) return <div className="attachment-file pending">{inner}</div>
  return (
    <a className="attachment-file" href={url} target="_blank" rel="noreferrer" download={att.filename}>
      {inner}
    </a>
  )
}
