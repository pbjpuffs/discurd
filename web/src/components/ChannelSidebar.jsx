import { useEffect, useRef, useState } from 'react'
import { useChatStore } from '../store/chatStore'
import { useAuthStore } from '../store/authStore'
import { useVoiceStore } from '../store/voiceStore'
import { ChevronDownIcon, CloseIcon, PlusIcon, VolumeIcon, MicOffIcon } from './Icons'
import Avatar from './Avatar'
import InviteModal from './InviteModal'
import CreateChannelModal from './CreateChannelModal'
import UserPanel from './UserPanel'
import VoiceControlBar from './VoiceControlBar'

export default function ChannelSidebar() {
  const me = useAuthStore((s) => s.user)
  const guild = useChatStore((s) => s.guilds.find((g) => g.id === s.selectedGuildId))
  const channels = useChatStore((s) => (s.selectedGuildId ? s.channelsByGuild[s.selectedGuildId] : null))
  const selectedChannelId = useChatStore((s) => s.selectedChannelId)
  const selectChannel = useChatStore((s) => s.selectChannel)
  const viewVoiceStage = useVoiceStore((s) => s.viewVoiceStage)
  const setViewingText = useVoiceStore((s) => s.setViewingText)
  const [menuOpen, setMenuOpen] = useState(false)
  const [showInvite, setShowInvite] = useState(false)
  const [showCreate, setShowCreate] = useState(false)
  const menuRef = useRef(null)
  const isOwner = !!(guild && me && guild.owner_id === me.id)

  useEffect(() => {
    if (!menuOpen) return
    const onDoc = (e) => {
      if (menuRef.current && !menuRef.current.contains(e.target)) setMenuOpen(false)
    }
    document.addEventListener('mousedown', onDoc)
    return () => document.removeEventListener('mousedown', onDoc)
  }, [menuOpen])

  const all = channels || []
  const textChannels = all.filter((c) => c.type !== 'voice')
  const voiceChannels = all.filter((c) => c.type === 'voice')

  const openTextChannel = (id) => {
    setViewingText()
    selectChannel(id)
  }

  return (
    <aside className="channel-sidebar">
      {guild ? (
        <>
          <div className="guild-header" ref={menuRef}>
            <button className="guild-header-btn" onClick={() => setMenuOpen((v) => !v)}>
              <span className="guild-header-name">{guild.name}</span>
              <span className="chevron">{menuOpen ? <CloseIcon size={16} /> : <ChevronDownIcon size={18} />}</span>
            </button>
            {menuOpen && (
              <div className="dropdown">
                <button
                  onClick={() => {
                    setMenuOpen(false)
                    setShowInvite(true)
                  }}
                >
                  Invite People
                </button>
                {isOwner && (
                  <button
                    onClick={() => {
                      setMenuOpen(false)
                      setShowCreate(true)
                    }}
                  >
                    Create Channel
                  </button>
                )}
              </div>
            )}
          </div>
          <div className="channel-list">
            <div className="channel-list-header">
              <span>Text Channels</span>
              {isOwner && (
                <button className="icon-btn" title="Create Channel" onClick={() => setShowCreate(true)}>
                  <PlusIcon size={16} />
                </button>
              )}
            </div>
            {textChannels.map((c) => (
              <button
                key={c.id}
                className={`channel-item ${c.id === selectedChannelId && !viewVoiceStage ? 'active' : ''}`}
                onClick={() => openTextChannel(c.id)}
              >
                <span className="hash">#</span>
                <span className="channel-item-name">{c.name}</span>
              </button>
            ))}

            {voiceChannels.length > 0 && (
              <>
                <div className="channel-list-header voice-header">
                  <span>Voice Channels</span>
                </div>
                {voiceChannels.map((c) => (
                  <VoiceChannelItem key={c.id} channel={c} guildId={guild.id} />
                ))}
              </>
            )}

            {all.length === 0 && <div className="sidebar-empty">No channels yet</div>}
          </div>
        </>
      ) : (
        <>
          <div className="guild-header">
            <span className="guild-header-name muted">Discurd</span>
          </div>
          <div className="channel-list" />
        </>
      )}
      <VoiceControlBar />
      <UserPanel />
      {showInvite && guild && <InviteModal guildId={guild.id} onClose={() => setShowInvite(false)} />}
      {showCreate && guild && <CreateChannelModal guildId={guild.id} onClose={() => setShowCreate(false)} />}
    </aside>
  )
}

function VoiceChannelItem({ channel, guildId }) {
  const join = useVoiceStore((s) => s.join)
  const connected = useVoiceStore((s) => s.connected)
  const connecting = useVoiceStore((s) => s.connecting)
  const roomChannelId = useVoiceStore((s) => s.roomChannelId)
  const viewVoiceStage = useVoiceStore((s) => s.viewVoiceStage)
  const participants = useVoiceStore((s) => s.participants)

  const isCurrent = roomChannelId === channel.id
  const inHere = isCurrent && (connected || connecting)
  const active = inHere && viewVoiceStage

  return (
    <div className="voice-channel">
      <button
        className={`channel-item voice ${active ? 'active' : ''} ${inHere ? 'joined' : ''}`}
        onClick={() => join(channel.id, guildId)}
      >
        <span className="hash">
          <VolumeIcon size={18} />
        </span>
        <span className="channel-item-name">{channel.name}</span>
        {connecting && isCurrent && <span className="voice-joining">…</span>}
      </button>
      {inHere && participants.length > 0 && (
        <div className="voice-participants">
          {participants.map((p) => (
            <div key={p.identity} className={`voice-participant ${p.isSpeaking && p.micEnabled ? 'speaking' : ''}`}>
              <span className="voice-participant-avatar">
                <Avatar name={p.name} url={p.avatarUrl} size={24} />
              </span>
              <span className="voice-participant-name">{p.name}</span>
              {!p.micEnabled && (
                <span className="voice-participant-muted" title="Muted">
                  <MicOffIcon size={14} />
                </span>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
