import { useEffect, useRef, useState } from 'react'
import { useAuthStore } from '../store/authStore'
import { useChatStore } from '../store/chatStore'
import { toast } from '../store/toastStore'
import Avatar from './Avatar'
import { formatTimestamp, formatTime, formatBytes } from '../lib/format'
import { PencilIcon, TrashIcon, FileIcon } from './Icons'

export default function MessageItem({ msg, grouped, guildOwnerId, channelId }) {
  const me = useAuthStore((s) => s.user)
  const editMessage = useChatStore((s) => s.editMessage)
  const deleteMessage = useChatStore((s) => s.deleteMessage)
  const dismissLocal = useChatStore((s) => s.dismissLocal)
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState('')
  const [busy, setBusy] = useState(false)
  const taRef = useRef(null)

  const isOwn = !!(me && msg.author.id === me.id)
  const canEdit = isOwn && !msg.optimistic
  const canDelete = (isOwn || (me && guildOwnerId === me.id)) && !msg.optimistic

  useEffect(() => {
    if (editing && taRef.current) {
      const ta = taRef.current
      ta.focus()
      ta.setSelectionRange(ta.value.length, ta.value.length)
    }
  }, [editing])

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

  const cls = ['message', grouped ? 'grouped' : 'first', msg.optimistic ? 'pending' : '', msg.failed ? 'failed' : '']
    .filter(Boolean)
    .join(' ')

  return (
    <div className={cls}>
      <div className="message-gutter">
        {grouped ? (
          <span className="hover-time">{formatTime(msg.created_at)}</span>
        ) : (
          <Avatar name={msg.author.username} url={msg.author.avatar_url} size={40} />
        )}
      </div>
      <div className="message-body">
        {!grouped && (
          <div className="message-meta">
            <span className="message-author">{msg.author.username}</span>
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
      {!editing && (canEdit || canDelete) && (
        <div className="message-actions">
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

// Only render attachment URLs that are our own uploaded objects. Never emit an
// attacker-controllable scheme (e.g. javascript:) into href/src, even though
// the server also validates this on message create — defense in depth.
function safeFileURL(url) {
  return typeof url === 'string' && url.startsWith('/files/') && !url.includes('..') ? url : null
}

function AttachmentView({ att, pending }) {
  const url = safeFileURL(att.url)
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
