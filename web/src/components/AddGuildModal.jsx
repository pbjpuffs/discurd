import { useState } from 'react'
import Modal from './Modal'
import { useChatStore } from '../store/chatStore'

export default function AddGuildModal({ onClose }) {
  const createGuild = useChatStore((s) => s.createGuild)
  const joinGuild = useChatStore((s) => s.joinGuild)
  const [tab, setTab] = useState('create')
  const [name, setName] = useState('')
  const [code, setCode] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const valid = tab === 'create' ? name.trim().length >= 2 : code.trim().length > 0

  const submit = async (e) => {
    e.preventDefault()
    if (!valid || busy) return
    setError('')
    setBusy(true)
    try {
      if (tab === 'create') await createGuild(name.trim())
      else await joinGuild(code.trim())
      onClose()
    } catch (err) {
      setError(err.message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal title={tab === 'create' ? 'Create a guild' : 'Join a guild'} onClose={onClose}>
      <div className="tabs">
        <button type="button" className={`tab ${tab === 'create' ? 'active' : ''}`} onClick={() => setTab('create')}>
          Create
        </button>
        <button type="button" className={`tab ${tab === 'join' ? 'active' : ''}`} onClick={() => setTab('join')}>
          Join with invite
        </button>
      </div>
      <form onSubmit={submit}>
        {tab === 'create' ? (
          <label className="field">
            <span>Guild name</span>
            <input
              autoFocus
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="My Cool Server"
              maxLength={100}
            />
          </label>
        ) : (
          <label className="field">
            <span>Invite code</span>
            <input autoFocus value={code} onChange={(e) => setCode(e.target.value)} placeholder="e.g. aB3xYz19" />
          </label>
        )}
        {error && <div className="form-error">{error}</div>}
        <div className="modal-actions">
          <button type="button" className="btn ghost" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" className="btn primary" disabled={!valid || busy}>
            {busy ? 'Working…' : tab === 'create' ? 'Create Guild' : 'Join Guild'}
          </button>
        </div>
      </form>
    </Modal>
  )
}
