import { useEffect, useState } from 'react'
import { useChatStore } from '../store/chatStore'
import { useVoiceStore } from '../store/voiceStore'
import { connectGateway, disconnectGateway } from '../lib/gateway'
import { toast } from '../store/toastStore'
import GuildRail from '../components/GuildRail'
import ChannelSidebar from '../components/ChannelSidebar'
import MessagePane from '../components/MessagePane'
import VoiceStage from '../components/VoiceStage'
import MemberList from '../components/MemberList'
import AddGuildModal from '../components/AddGuildModal'

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

  return (
    <div className="app-shell">
      {guildsLoaded && !wsConnected && <div className="reconnect-bar">Connecting…</div>}
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
