import { useState } from 'react'
import Modal from './Modal'
import { useChatStore } from '../store/chatStore'
import { HashIcon, VolumeIcon } from './Icons'

export default function CreateChannelModal({ guildId, onClose }) {
  const createChannel = useChatStore((s) => s.createChannel)
  const [name, setName] = useState('')
  const [topic, setTopic] = useState('')
  const [type, setType] = useState('text')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const valid = name.trim().length >= 1

  const submit = async (e) => {
    e.preventDefault()
    if (!valid || busy) return
    setError('')
    setBusy(true)
    try {
      await createChannel(guildId, name.trim(), topic.trim(), type)
      onClose()
    } catch (err) {
      setError(err.message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal title="Create Channel" onClose={onClose}>
      <form onSubmit={submit}>
        <div className="field">
          <span>Channel type</span>
          <div className="channel-type-select">
            <button
              type="button"
              className={`channel-type-option ${type === 'text' ? 'active' : ''}`}
              onClick={() => setType('text')}
            >
              <HashIcon size={22} />
              <span className="channel-type-label">Text</span>
              <span className="channel-type-desc">Send messages, images, and files</span>
            </button>
            <button
              type="button"
              className={`channel-type-option ${type === 'voice' ? 'active' : ''}`}
              onClick={() => setType('voice')}
            >
              <VolumeIcon size={22} />
              <span className="channel-type-label">Voice</span>
              <span className="channel-type-desc">Talk with voice, video, and screen share</span>
            </button>
          </div>
        </div>
        <label className="field">
          <span>Channel name</span>
          <input
            autoFocus
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={type === 'voice' ? 'General' : 'new-channel'}
            maxLength={100}
          />
        </label>
        <label className="field">
          <span>Topic (optional)</span>
          <input value={topic} onChange={(e) => setTopic(e.target.value)} placeholder="What's this channel about?" maxLength={1024} />
        </label>
        {error && <div className="form-error">{error}</div>}
        <div className="modal-actions">
          <button type="button" className="btn ghost" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" className="btn primary" disabled={!valid || busy}>
            {busy ? 'Creating…' : 'Create Channel'}
          </button>
        </div>
      </form>
    </Modal>
  )
}
