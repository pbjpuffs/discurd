import { useEffect } from 'react'
import { useChatStore } from '../store/chatStore'

export default function TypingIndicator({ channelId }) {
  const typing = useChatStore((s) => s.typingByChannel[channelId])

  const active = typing ? Object.values(typing).filter((v) => v.until > Date.now()) : []

  useEffect(() => {
    if (!typing || Object.keys(typing).length === 0) return undefined
    const t = setInterval(() => useChatStore.getState().pruneTyping(channelId), 1000)
    return () => clearInterval(t)
  }, [typing, channelId])

  const names = active.map((v) => v.username)

  return (
    <div className="typing-bar">
      {names.length > 0 && (
        <>
          <span className="typing-dots">
            <span />
            <span />
            <span />
          </span>
          <span className="typing-text">
            {names.length === 1 ? (
              <>
                <b>{names[0]}</b> is typing…
              </>
            ) : names.length <= 3 ? (
              <>
                <b>{names.join(', ')}</b> are typing…
              </>
            ) : (
              'Several people are typing…'
            )}
          </span>
        </>
      )}
    </div>
  )
}
