import { useEffect, useRef, useState } from 'react'
import { useChatStore } from '../store/chatStore'
import { useVoiceStore } from '../store/voiceStore'
import { useEffectsStore } from '../store/effectsStore'
import { useUiStore } from '../store/uiStore'
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
import MobileTopBar from '../components/MobileTopBar'

const SWIPE_THRESHOLD = 50

export default function AppShell() {
  const guildsLoaded = useChatStore((s) => s.guildsLoaded)
  const hasGuilds = useChatStore((s) => s.guilds.length > 0)
  const wsConnected = useChatStore((s) => s.wsConnected)
  const loadGuilds = useChatStore((s) => s.loadGuilds)
  const selectedGuildId = useChatStore((s) => s.selectedGuildId)
  const selectedChannelId = useChatStore((s) => s.selectedChannelId)
  const voiceView = useVoiceStore((s) => s.viewVoiceStage && (s.connected || s.connecting))
  const navOpen = useUiStore((s) => s.navOpen)
  const membersOpen = useUiStore((s) => s.membersOpen)
  const closeDrawers = useUiStore((s) => s.closeDrawers)
  const [showAdd, setShowAdd] = useState(false)

  const noGuilds = guildsLoaded && !hasGuilds

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

  // Escape closes any open drawer.
  useEffect(() => {
    const onKey = (e) => {
      if (e.key === 'Escape') closeDrawers()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [closeDrawers])

  // Close drawers when the active guild/channel changes (navigating settles on
  // the message pane on mobile).
  useEffect(() => {
    closeDrawers()
  }, [selectedGuildId, selectedChannelId, closeDrawers])

  // Swipe-to-close for the mobile drawers: track the touch start and, on lift,
  // close if the gesture was a decisive horizontal swipe toward the screen edge.
  const touchRef = useRef(null)
  const onTouchStart = (e) => {
    const t = e.touches[0]
    touchRef.current = { x: t.clientX, y: t.clientY }
  }
  const swipeClose = (e, direction) => {
    const start = touchRef.current
    touchRef.current = null
    if (!start) return
    const t = e.changedTouches[0]
    const dx = t.clientX - start.x
    const dy = t.clientY - start.y
    if (Math.abs(dx) < SWIPE_THRESHOLD || Math.abs(dx) <= Math.abs(dy)) return
    if ((direction === 'left' && dx < 0) || (direction === 'right' && dx > 0)) closeDrawers()
  }

  return (
    <div className="app-shell">
      {guildsLoaded && !wsConnected && <div className="reconnect-bar">Connecting…</div>}
      <EffectsOverlay />
      <ProfileCard />
      <LightningButton />
      <MobileTopBar />
      <div
        className={`nav-drawer ${navOpen ? 'open' : ''}`}
        onTouchStart={onTouchStart}
        onTouchEnd={(e) => swipeClose(e, 'left')}
      >
        <GuildRail />
        {!noGuilds && <ChannelSidebar />}
      </div>
      {noGuilds ? (
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
          {voiceView ? <VoiceStage /> : <MessagePane />}
          <div
            className={`members-drawer ${membersOpen ? 'open' : ''}`}
            onTouchStart={onTouchStart}
            onTouchEnd={(e) => swipeClose(e, 'right')}
          >
            <MemberList />
          </div>
        </>
      )}
      {(navOpen || membersOpen) && <div className="drawer-backdrop" onClick={closeDrawers} />}
    </div>
  )
}
