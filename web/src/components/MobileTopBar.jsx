import { useChatStore } from '../store/chatStore'
import { useVoiceStore } from '../store/voiceStore'
import { useUiStore } from '../store/uiStore'
import { MenuIcon, PeopleIcon, HashIcon, VolumeIcon } from './Icons'

// Mobile-only app bar. Hidden on desktop (see responsive CSS). Provides the two
// drawer toggles (☰ nav / people) and shows the current channel (or guild) name.
export default function MobileTopBar() {
  const toggleNav = useUiStore((s) => s.toggleNav)
  const toggleMembers = useUiStore((s) => s.toggleMembers)
  const guild = useChatStore((s) => s.guilds.find((g) => g.id === s.selectedGuildId))
  const voiceView = useVoiceStore((s) => s.viewVoiceStage && (s.connected || s.connecting))
  const roomChannelId = useVoiceStore((s) => s.roomChannelId)
  const roomGuildId = useVoiceStore((s) => s.roomGuildId)
  const textChannel = useChatStore((s) =>
    s.selectedGuildId && s.selectedChannelId
      ? (s.channelsByGuild[s.selectedGuildId] || []).find((c) => c.id === s.selectedChannelId)
      : null,
  )
  const voiceChannel = useChatStore((s) =>
    roomGuildId ? (s.channelsByGuild[roomGuildId] || []).find((c) => c.id === roomChannelId) : null,
  )

  const channel = voiceView ? voiceChannel : textChannel
  const title = channel ? channel.name : guild ? guild.name : 'Discurd'
  const isVoice = voiceView && !!voiceChannel

  return (
    <header className="mobile-top-bar">
      <button className="icon-btn topbar-btn" onClick={toggleNav} aria-label="Open navigation" title="Menu">
        <MenuIcon size={24} />
      </button>
      <span className="topbar-title">
        <span className="topbar-hash">{isVoice ? <VolumeIcon size={18} /> : <HashIcon size={18} />}</span>
        <span className="topbar-name">{title}</span>
      </span>
      {guild ? (
        <button className="icon-btn topbar-btn" onClick={toggleMembers} aria-label="Show members" title="Members">
          <PeopleIcon size={24} />
        </button>
      ) : (
        <span className="topbar-btn" aria-hidden="true" />
      )}
    </header>
  )
}
