import { useChatStore } from '../store/chatStore'
import Avatar from './Avatar'
import { CrownIcon } from './Icons'

const byName = (a, b) => a.username.localeCompare(b.username)

export default function MemberList() {
  const guild = useChatStore((s) => s.guilds.find((g) => g.id === s.selectedGuildId))
  const members = useChatStore((s) => (s.selectedGuildId ? s.membersByGuild[s.selectedGuildId] : null))
  if (!guild) return null

  const list = members || []
  const online = list.filter((m) => m.status === 'online').sort(byName)
  const offline = list.filter((m) => m.status !== 'online').sort(byName)

  return (
    <aside className="member-list" aria-label="Members">
      <MemberGroup label={`Online — ${online.length}`} members={online} />
      <MemberGroup label={`Offline — ${offline.length}`} members={offline} offline />
    </aside>
  )
}

function MemberGroup({ label, members, offline = false }) {
  if (members.length === 0) return null
  return (
    <div className="member-group">
      <div className="member-group-label">{label}</div>
      {members.map((m) => (
        <div key={m.user_id} className={`member-row ${offline ? 'offline' : ''}`}>
          <span className="member-avatar">
            <Avatar name={m.username} url={m.avatar_url} size={32} />
            <span className={`presence-dot ${m.status === 'online' ? 'online' : 'off'}`} />
          </span>
          <span className="member-name" title={m.username}>
            {m.username}
          </span>
          {m.is_owner && (
            <span className="owner-crown" title="Guild Owner">
              <CrownIcon size={14} />
            </span>
          )}
        </div>
      ))}
    </div>
  )
}
