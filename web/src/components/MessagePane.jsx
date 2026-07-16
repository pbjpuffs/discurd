import { useLayoutEffect, useRef } from 'react'
import { useChatStore } from '../store/chatStore'
import MessageItem from './MessageItem'
import MessageInput from './MessageInput'
import TypingIndicator from './TypingIndicator'
import { HashIcon } from './Icons'

const GROUP_WINDOW_MS = 5 * 60 * 1000

export default function MessagePane() {
  const guild = useChatStore((s) => s.guilds.find((g) => g.id === s.selectedGuildId))
  const channel = useChatStore((s) =>
    s.selectedGuildId && s.selectedChannelId
      ? (s.channelsByGuild[s.selectedGuildId] || []).find((c) => c.id === s.selectedChannelId)
      : null,
  )
  const conv = useChatStore((s) => (s.selectedChannelId ? s.messagesByChannel[s.selectedChannelId] : null))
  const loadOlder = useChatStore((s) => s.loadOlder)

  const scrollRef = useRef(null)
  const stickBottomRef = useRef(true)
  const prependRef = useRef(null)
  const channelId = channel ? channel.id : null
  const items = conv ? conv.items : null

  // New channel: reset scroll behavior to "pinned to bottom".
  useLayoutEffect(() => {
    stickBottomRef.current = true
    prependRef.current = null
  }, [channelId])

  // After any message-list change: either restore position across an upward
  // prepend, or keep the view pinned to the bottom.
  useLayoutEffect(() => {
    const el = scrollRef.current
    if (!el) return
    if (prependRef.current !== null) {
      const { top, height } = prependRef.current
      prependRef.current = null
      el.scrollTop = top + (el.scrollHeight - height)
    } else if (stickBottomRef.current) {
      el.scrollTop = el.scrollHeight
    }
  }, [items, channelId])

  const onScroll = () => {
    const el = scrollRef.current
    if (!el || !channelId) return
    stickBottomRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80
    const cur = useChatStore.getState().messagesByChannel[channelId]
    if (el.scrollTop < 150 && cur && cur.loaded && cur.hasMore && !cur.loadingOlder) {
      prependRef.current = { top: el.scrollTop, height: el.scrollHeight }
      loadOlder(channelId)
    }
  }

  if (!guild) {
    return (
      <main className="message-pane empty-pane">
        <div className="empty-state">
          <h3>No guild selected</h3>
          <p>Pick a guild on the left, or create one with the + button.</p>
        </div>
      </main>
    )
  }
  if (!channel) {
    return (
      <main className="message-pane empty-pane">
        <div className="empty-state">
          <h3>No channel selected</h3>
          <p>{(useChatStore.getState().channelsByGuild[guild.id] || []).length === 0 ? 'This guild has no channels yet.' : 'Select a channel to start chatting.'}</p>
        </div>
      </main>
    )
  }

  return (
    <main className="message-pane">
      <header className="channel-header">
        <span className="channel-header-hash">
          <HashIcon size={22} />
        </span>
        <span className="channel-title">{channel.name}</span>
        {channel.topic && (
          <>
            <span className="header-divider" />
            <span className="channel-topic" title={channel.topic}>
              {channel.topic}
            </span>
          </>
        )}
      </header>
      <div className="messages-scroll" ref={scrollRef} onScroll={onScroll}>
        <div className="messages-inner">
          {conv && conv.loaded && !conv.hasMore && (
            <div className="channel-start">
              <div className="channel-start-icon">
                <HashIcon size={36} />
              </div>
              <h3>Welcome to #{channel.name}!</h3>
              <p>This is the start of the #{channel.name} channel.</p>
            </div>
          )}
          {conv && conv.loadingOlder && <div className="messages-loading">Loading older messages…</div>}
          {(items || []).map((m, i) => {
            const prev = i > 0 ? items[i - 1] : null
            const grouped =
              !!prev &&
              prev.author.id === m.author.id &&
              !prev.failed &&
              new Date(m.created_at) - new Date(prev.created_at) < GROUP_WINDOW_MS
            return (
              <MessageItem key={m.id} msg={m} grouped={grouped} guildOwnerId={guild.owner_id} channelId={channel.id} />
            )
          })}
          {conv && !conv.loaded && <div className="messages-loading">Loading messages…</div>}
        </div>
      </div>
      <MessageInput key={channel.id} channelId={channel.id} guildId={guild.id} channelName={channel.name} />
      <TypingIndicator channelId={channel.id} />
    </main>
  )
}
