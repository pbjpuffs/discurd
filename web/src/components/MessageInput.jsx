import { useRef, useState } from 'react'
import { useChatStore } from '../store/chatStore'
import { api } from '../lib/api'
import { PaperclipIcon, CloseIcon } from './Icons'

const MAX_LEN = 4000
const COUNTER_THRESHOLD = MAX_LEN - 400
const TYPING_THROTTLE_MS = 8000
const MAX_FILES = 10

export default function MessageInput({ channelId, guildId, channelName }) {
  const sendMessage = useChatStore((s) => s.sendMessage)
  const [text, setText] = useState('')
  const [files, setFiles] = useState([])
  const fileRef = useRef(null)
  const taRef = useRef(null)
  const lastTypingRef = useRef(0)

  const autoResize = () => {
    const ta = taRef.current
    if (!ta) return
    ta.style.height = 'auto'
    ta.style.height = `${Math.min(ta.scrollHeight, 200)}px`
  }

  const onChange = (e) => {
    setText(e.target.value)
    autoResize()
    if (e.target.value.trim() && Date.now() - lastTypingRef.current > TYPING_THROTTLE_MS) {
      lastTypingRef.current = Date.now()
      api.post(`/channels/${channelId}/typing`).catch(() => {})
    }
  }

  const canSend = (text.trim().length > 0 || files.length > 0) && text.length <= MAX_LEN

  const send = () => {
    if (!canSend) return
    const content = text.trim()
    const toSend = files
    setText('')
    setFiles([])
    lastTypingRef.current = 0
    if (taRef.current) taRef.current.style.height = 'auto'
    sendMessage(channelId, guildId, content, toSend)
  }

  const onKeyDown = (e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      send()
    }
  }

  const onPickFiles = (e) => {
    const picked = Array.from(e.target.files || [])
    e.target.value = ''
    if (picked.length > 0) setFiles((f) => [...f, ...picked].slice(0, MAX_FILES))
  }

  const remaining = MAX_LEN - text.length

  return (
    <div className="message-input-area">
      {files.length > 0 && (
        <div className="upload-chips">
          {files.map((f, i) => (
            <div key={`${f.name}-${i}`} className="upload-chip">
              <span className="file-name">{f.name}</span>
              <button
                className="icon-btn"
                title="Remove attachment"
                onClick={() => setFiles(files.filter((_, j) => j !== i))}
              >
                <CloseIcon size={14} />
              </button>
            </div>
          ))}
        </div>
      )}
      <div className="message-input">
        <button className="icon-btn attach" title="Attach files" onClick={() => fileRef.current && fileRef.current.click()}>
          <PaperclipIcon />
        </button>
        <input ref={fileRef} type="file" multiple hidden onChange={onPickFiles} />
        <textarea
          ref={taRef}
          rows={1}
          placeholder={`Message #${channelName}`}
          value={text}
          onChange={onChange}
          onKeyDown={onKeyDown}
        />
        {text.length >= COUNTER_THRESHOLD && (
          <span className={`char-counter ${remaining < 0 ? 'over' : ''}`}>{remaining}</span>
        )}
      </div>
    </div>
  )
}
