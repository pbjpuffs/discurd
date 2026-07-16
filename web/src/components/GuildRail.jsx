import { useState } from 'react'
import { useChatStore } from '../store/chatStore'
import { PlusIcon } from './Icons'
import AddGuildModal from './AddGuildModal'

export default function GuildRail() {
  const guilds = useChatStore((s) => s.guilds)
  const selectedGuildId = useChatStore((s) => s.selectedGuildId)
  const selectGuild = useChatStore((s) => s.selectGuild)
  const [showAdd, setShowAdd] = useState(false)

  return (
    <nav className="guild-rail" aria-label="Guilds">
      {guilds.map((g) => (
        <button
          key={g.id}
          title={g.name}
          className={`guild-icon ${g.id === selectedGuildId ? 'active' : ''}`}
          onClick={() => selectGuild(g.id)}
        >
          <span className="guild-pill" />
          {g.icon_url ? <img src={g.icon_url} alt="" /> : <span className="guild-letter">{g.name.charAt(0).toUpperCase()}</span>}
        </button>
      ))}
      <button className="guild-icon add-guild" title="Add a guild" onClick={() => setShowAdd(true)}>
        <PlusIcon size={22} />
      </button>
      {showAdd && <AddGuildModal onClose={() => setShowAdd(false)} />}
    </nav>
  )
}
