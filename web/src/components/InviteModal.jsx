import { useEffect, useState } from 'react'
import Modal from './Modal'
import { useChatStore } from '../store/chatStore'

export default function InviteModal({ guildId, onClose }) {
  const createInvite = useChatStore((s) => s.createInvite)
  const [invite, setInvite] = useState(null)
  const [error, setError] = useState('')
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    let cancelled = false
    createInvite(guildId)
      .then((inv) => {
        if (!cancelled) setInvite(inv)
      })
      .catch((e) => {
        if (!cancelled) setError(e.message)
      })
    return () => {
      cancelled = true
    }
  }, [guildId, createInvite])

  const copy = async () => {
    if (!invite) return
    try {
      await navigator.clipboard.writeText(invite.code)
    } catch {
      const ta = document.createElement('textarea')
      ta.value = invite.code
      document.body.appendChild(ta)
      ta.select()
      document.execCommand('copy')
      ta.remove()
    }
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }

  return (
    <Modal title="Invite People" onClose={onClose}>
      <p className="modal-hint">
        Share this invite code — anyone can join from the <b>+</b> button in their guild rail.
      </p>
      {error && <div className="form-error">{error}</div>}
      <div className="invite-row">
        <code className="invite-code">{invite ? invite.code : '········'}</code>
        <button className="btn primary" onClick={copy} disabled={!invite}>
          {copied ? 'Copied!' : 'Copy'}
        </button>
      </div>
    </Modal>
  )
}
