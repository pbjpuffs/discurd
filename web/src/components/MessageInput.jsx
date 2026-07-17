import { useEffect, useRef, useState } from 'react'
import { useChatStore } from '../store/chatStore'
import { useGifStore } from '../store/gifStore'
import { useEffectsStore } from '../store/effectsStore'
import { api } from '../lib/api'
import EmojiPicker from './EmojiPicker'
import GifPicker from './GifPicker'
import { PaperclipIcon, CloseIcon, SmileIcon, GifIcon, BoltIcon } from './Icons'
import { EFFECT_TYPES, EFFECT_LABELS } from '../lib/effects'

const MAX_LEN = 4000
const COUNTER_THRESHOLD = MAX_LEN - 400
const TYPING_THROTTLE_MS = 8000
const MAX_FILES = 10

export default function MessageInput({ channelId, guildId, channelName }) {
  const sendMessage = useChatStore((s) => s.sendMessage)
  const sendGif = useChatStore((s) => s.sendGif)
  const gifAvailable = useGifStore((s) => s.available)
  const probeGifs = useGifStore((s) => s.probe)
  const triggerEffect = useEffectsStore((s) => s.trigger)
  const stormMode = useEffectsStore((s) => s.stormMode)
  const toggleStorm = useEffectsStore((s) => s.toggleStorm)
  const [text, setText] = useState('')
  const [files, setFiles] = useState([])
  const [openPanel, setOpenPanel] = useState(null) // 'emoji' | 'gif' | 'fx' | null
  const fileRef = useRef(null)
  const taRef = useRef(null)
  const areaRef = useRef(null)
  const lastTypingRef = useRef(0)

  useEffect(() => {
    probeGifs()
  }, [probeGifs])

  // Close any open composer panel on outside click.
  useEffect(() => {
    if (!openPanel) return undefined
    const onDown = (e) => {
      if (areaRef.current && !areaRef.current.contains(e.target)) setOpenPanel(null)
    }
    document.addEventListener('mousedown', onDown)
    return () => document.removeEventListener('mousedown', onDown)
  }, [openPanel])

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

  // Insert an emoji at the caret without closing the picker (multi-insert).
  const insertEmoji = (native) => {
    const ta = taRef.current
    const cur = ta ? ta.value : text
    const start = ta ? ta.selectionStart : cur.length
    const end = ta ? ta.selectionEnd : cur.length
    const next = cur.slice(0, start) + native + cur.slice(end)
    setText(next)
    requestAnimationFrame(() => {
      if (!ta) return
      ta.focus()
      const p = start + native.length
      ta.setSelectionRange(p, p)
      autoResize()
    })
  }

  const onPickGif = (gif) => {
    setOpenPanel(null)
    sendGif(channelId, guildId, gif)
  }

  const fireEffect = (type) => {
    triggerEffect(type)
    setOpenPanel(null)
  }

  const togglePanel = (name) => setOpenPanel((cur) => (cur === name ? null : name))

  const remaining = MAX_LEN - text.length

  return (
    <div className="message-input-area" ref={areaRef}>
      {/* Dim backdrop behind the bottom-sheet popovers on mobile (hidden on desktop). */}
      {openPanel && <div className="sheet-backdrop" onMouseDown={() => setOpenPanel(null)} />}
      {openPanel === 'emoji' && (
        <div className="composer-pop emoji">
          <EmojiPicker onPick={insertEmoji} />
        </div>
      )}
      {openPanel === 'gif' && (
        <div className="composer-pop gif">
          <GifPicker onPick={onPickGif} />
        </div>
      )}
      {openPanel === 'fx' && (
        <div className="composer-pop fx effects-menu" onMouseDown={(e) => e.stopPropagation()}>
          <div className="effects-menu-title">Effects</div>
          <div className="effects-menu-grid">
            {EFFECT_TYPES.map((type) => (
              <button key={type} className="effects-menu-item" onClick={() => fireEffect(type)}>
                {EFFECT_LABELS[type]}
              </button>
            ))}
          </div>
          <label className="storm-toggle">
            <span>Storm Mode</span>
            <button
              type="button"
              className={`switch ${stormMode ? 'on' : ''}`}
              role="switch"
              aria-checked={stormMode}
              onClick={toggleStorm}
            >
              <span className="switch-knob" />
            </button>
          </label>
        </div>
      )}

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
        <button
          className="icon-btn attach"
          title="Attach files"
          onClick={() => fileRef.current && fileRef.current.click()}
        >
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
        <div className="composer-tools">
          {gifAvailable !== false && (
            <button
              className={`icon-btn composer-tool ${openPanel === 'gif' ? 'active' : ''}`}
              title="Send a GIF"
              onClick={() => togglePanel('gif')}
            >
              <GifIcon size={22} />
            </button>
          )}
          <button
            className={`icon-btn composer-tool ${openPanel === 'fx' ? 'active' : ''}`}
            title="Effects"
            onClick={() => togglePanel('fx')}
          >
            <BoltIcon size={20} />
          </button>
          <button
            className={`icon-btn composer-tool ${openPanel === 'emoji' ? 'active' : ''}`}
            title="Emoji"
            onClick={() => togglePanel('emoji')}
          >
            <SmileIcon size={22} />
          </button>
        </div>
      </div>
    </div>
  )
}
