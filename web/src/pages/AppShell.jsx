import { useEffect, useState } from 'react'
import { useChatStore } from '../store/chatStore'
import { useVoiceStore } from '../store/voiceStore'
import { useEffectsStore } from '../store/effectsStore'
import { connectGateway, disconnectGateway } from '../lib/gateway'
import { toast } from '../store/toastStore'
import GuildRail from '../components/GuildRail'
import ChannelSidebar from '../components/ChannelSidebar'
import MessagePane from '../components/MessagePane'
import VoiceStage from '../components/VoiceStage'
import MemberList from '../components/MemberList'
import AddGuildModal from '../components/AddGuildModal'
import EffectsOverlay from '../components/EffectsOverlay'
import LightningButton from '../components/LightningButton'
import ProfileCard from '../components/ProfileCard'

export default function AppShell() {
  const guildsLoaded = useChatStore((s) => s.guildsLoaded)
  const hasGuilds = useChatStore((s) => s.guilds.length > 0)
  const wsConnected = useChatStore((s) => s.wsConnected)
  const loadGuilds = useChatStore((s) => s.loadGuilds)
  const voiceView = useVoiceStore((s) => s.viewVoiceStage && (s.connected || s.connecting))
  const [showAdd, setShowAdd] = useState(false)

  useEffect(() => {
    loadGuilds().catch((e) => toast(e.message))
    connectGateway()
    return () => disconnectGateway()
  }, [loadGuilds])

  // Global keyboard shortcut: Alt+L strikes (and broadcasts) lightning. Chosen
  // to avoid clobbering browser essentials (Ctrl/Cmd+L is the address bar).
  useEffect(() => {
    const onKey = (e) => {
      if (e.altKey && !e.ctrlKey && !e.metaKey && (e.key === 'l' || e.key === 'L')) {
        const t = e.target
        const tag = t && t.tagName
        if (tag === 'INPUT' || tag === 'TEXTAREA' || (t && t.isContentEditable)) return
        e.preventDefault()
        useEffectsStore.getState().trigger('lightning')
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  return (
    <div className="app-shell">
      {guildsLoaded && !wsConnected && <div className="reconnect-bar">Connecting…</div>}
      <EffectsOverlay />
      <ProfileCard />
      <LightningButton />
      <GuildRail />
      {guildsLoaded && !hasGuilds ? (
        <div className="no-guilds">
          <h2>Welcome to Discurd</h2>
          <p>You're not in any guilds yet. Create your own, or join one with an invite code.</p>
          <button className="btn primary" onClick={() => setShowAdd(true)}>
            Create or join a guild
          </button>
          {showAdd && <AddGuildModal onClose={() => setShowAdd(false)} />}
        </div>
      ) : (
        <>
          <ChannelSidebar />
          {voiceView ? <VoiceStage /> : <MessagePane />}
          <MemberList />
        </>
      )}
    </div>
  )
}
