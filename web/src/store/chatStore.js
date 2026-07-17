import { create } from 'zustand'
import { api } from '../lib/api'
import { useAuthStore } from './authStore'
import { useEffectsStore } from './effectsStore'
import { toast } from './toastStore'

const PAGE_SIZE = 50
const TYPING_TTL_MS = 10000
let tempCounter = 0

function freshConv(msgsNewestFirst) {
  return {
    items: msgsNewestFirst.slice().reverse(), // stored oldest -> newest
    hasMore: msgsNewestFirst.length >= PAGE_SIZE,
    loaded: true,
    loadingInitial: false,
    loadingOlder: false,
  }
}

function withConv(state, channelId, update) {
  const conv = state.messagesByChannel[channelId]
  if (!conv) return {}
  return { messagesByChannel: { ...state.messagesByChannel, [channelId]: { ...conv, ...update(conv) } } }
}

export const useChatStore = create((set, get) => ({
  wsConnected: false,
  guilds: [],
  guildsLoaded: false,
  selectedGuildId: null,
  selectedChannelId: null,
  lastChannelByGuild: {},
  channelsByGuild: {},
  membersByGuild: {},
  messagesByChannel: {},
  typingByChannel: {},

  setWsConnected: (v) => set({ wsConnected: v }),

  reset: () =>
    set({
      wsConnected: false,
      guilds: [],
      guildsLoaded: false,
      selectedGuildId: null,
      selectedChannelId: null,
      lastChannelByGuild: {},
      channelsByGuild: {},
      membersByGuild: {},
      messagesByChannel: {},
      typingByChannel: {},
    }),

  // ---- loading / selection ----

  loadGuilds: async () => {
    const guilds = await api.get('/users/@me/guilds')
    set({ guilds, guildsLoaded: true })
    const { selectedGuildId } = get()
    if (!selectedGuildId && guilds.length > 0) {
      await get().selectGuild(guilds[0].id)
    } else if (selectedGuildId && !guilds.some((g) => g.id === selectedGuildId)) {
      set({ selectedGuildId: null, selectedChannelId: null })
    }
  },

  selectGuild: async (guildId) => {
    set({ selectedGuildId: guildId })
    try {
      let channels = get().channelsByGuild[guildId]
      if (!channels) {
        channels = await api.get(`/guilds/${guildId}/channels`)
        set((s) => ({ channelsByGuild: { ...s.channelsByGuild, [guildId]: channels } }))
      }
      if (get().selectedGuildId !== guildId) return // user navigated away meanwhile
      const remembered = get().lastChannelByGuild[guildId]
      // Default to a text channel — voice channels have no message pane, so we
      // never want the message view pointed at one.
      const firstText = channels.find((c) => c.type !== 'voice')
      const target = channels.some((c) => c.id === remembered && c.type !== 'voice')
        ? remembered
        : firstText
          ? firstText.id
          : null
      get().selectChannel(target)
      if (!get().membersByGuild[guildId]) await get().loadMembers(guildId)
    } catch (e) {
      toast(e.message)
    }
  },

  selectChannel: (channelId) => {
    set((s) => ({
      selectedChannelId: channelId,
      lastChannelByGuild:
        channelId && s.selectedGuildId
          ? { ...s.lastChannelByGuild, [s.selectedGuildId]: channelId }
          : s.lastChannelByGuild,
    }))
    if (channelId) get().loadMessages(channelId).catch((e) => toast(e.message))
  },

  loadMembers: async (guildId) => {
    const members = await api.get(`/guilds/${guildId}/members`)
    set((s) => ({ membersByGuild: { ...s.membersByGuild, [guildId]: members } }))
  },

  loadMessages: async (channelId) => {
    const existing = get().messagesByChannel[channelId]
    if (existing && (existing.loaded || existing.loadingInitial)) return
    set((s) => ({
      messagesByChannel: {
        ...s.messagesByChannel,
        [channelId]: { items: [], hasMore: false, loaded: false, loadingInitial: true, loadingOlder: false, pending: [] },
      },
    }))
    try {
      const msgs = await api.get(`/channels/${channelId}/messages?limit=${PAGE_SIZE}`)
      set((s) => {
        const prev = s.messagesByChannel[channelId]
        const conv = freshConv(msgs)
        // Merge any MESSAGE_CREATEs that arrived over the WS while this fetch
        // was in flight — otherwise a message created after the server's read
        // snapshot but before the response lands would be silently dropped.
        const buffered = (prev && prev.pending) || []
        if (buffered.length) {
          const seen = new Set(conv.items.map((m) => m.id))
          for (const m of buffered) if (!seen.has(m.id)) conv.items.push(m)
        }
        return { messagesByChannel: { ...s.messagesByChannel, [channelId]: conv } }
      })
    } catch (e) {
      set((s) => {
        const next = { ...s.messagesByChannel }
        delete next[channelId]
        return { messagesByChannel: next }
      })
      throw e
    }
  },

  loadOlder: async (channelId) => {
    const conv = get().messagesByChannel[channelId]
    if (!conv || !conv.loaded || !conv.hasMore || conv.loadingOlder) return
    const oldest = conv.items.find((m) => !m.optimistic)
    if (!oldest) return
    set((s) => withConv(s, channelId, () => ({ loadingOlder: true })))
    try {
      const older = await api.get(`/channels/${channelId}/messages?limit=${PAGE_SIZE}&before=${oldest.id}`)
      set((s) =>
        withConv(s, channelId, (cur) => ({
          items: [...older.slice().reverse(), ...cur.items],
          hasMore: older.length >= PAGE_SIZE,
          loadingOlder: false,
        })),
      )
    } catch (e) {
      set((s) => withConv(s, channelId, () => ({ loadingOlder: false })))
      toast(e.message)
    }
  },

  // ---- sending / editing / deleting ----

  sendMessage: async (channelId, guildId, content, files) => {
    const user = useAuthStore.getState().user
    const tempId = `pending-${Date.now()}-${++tempCounter}`
    const optimistic = {
      id: tempId,
      channel_id: channelId,
      guild_id: guildId,
      author: { id: user.id, username: user.username, avatar_url: user.avatar_url || '' },
      content,
      attachments: files.map((f) => ({
        url: '',
        filename: f.name,
        size: f.size,
        content_type: f.type || 'application/octet-stream',
      })),
      reactions: [],
      created_at: new Date().toISOString(),
      edited_at: null,
      optimistic: true,
    }
    set((s) => withConv(s, channelId, (cur) => ({ items: [...cur.items, optimistic] })))
    try {
      const attachments = []
      for (const f of files) {
        attachments.push(await api.upload(`/channels/${channelId}/attachments`, f))
      }
      const body = { content }
      if (attachments.length > 0) body.attachments = attachments
      const msg = await api.post(`/channels/${channelId}/messages`, body)
      get().resolveOptimistic(channelId, tempId, msg)
    } catch (e) {
      set((s) =>
        withConv(s, channelId, (cur) => ({
          items: cur.items.map((m) => (m.id === tempId ? { ...m, failed: true } : m)),
        })),
      )
      toast(e.message)
    }
  },

  // Replace the optimistic entry with the server echo; if the WS dispatch
  // already delivered the real message, just drop the optimistic one.
  resolveOptimistic: (channelId, tempId, msg) =>
    set((s) =>
      withConv(s, channelId, (cur) => ({
        items: cur.items.some((m) => m.id === msg.id)
          ? cur.items.filter((m) => m.id !== tempId)
          : cur.items.map((m) => (m.id === tempId ? msg : m)),
      })),
    ),

  dismissLocal: (channelId, id) =>
    set((s) => withConv(s, channelId, (cur) => ({ items: cur.items.filter((m) => m.id !== id) }))),

  // Send a picked GIF as an external image/gif attachment (no upload — the URL
  // is a Tenor/Giphy media URL). Reuses the optimistic + resolve machinery.
  sendGif: async (channelId, guildId, gif) => {
    const user = useAuthStore.getState().user
    const attachment = { url: gif.url, filename: 'gif', size: 0, content_type: 'image/gif' }
    const tempId = `pending-${Date.now()}-${++tempCounter}`
    const optimistic = {
      id: tempId,
      channel_id: channelId,
      guild_id: guildId,
      author: { id: user.id, username: user.username, avatar_url: user.avatar_url || '' },
      content: '',
      attachments: [attachment],
      reactions: [],
      created_at: new Date().toISOString(),
      edited_at: null,
      optimistic: true,
    }
    set((s) => withConv(s, channelId, (cur) => ({ items: [...cur.items, optimistic] })))
    try {
      const msg = await api.post(`/channels/${channelId}/messages`, { content: '', attachments: [attachment] })
      get().resolveOptimistic(channelId, tempId, msg)
    } catch (e) {
      set((s) =>
        withConv(s, channelId, (cur) => ({
          items: cur.items.map((m) => (m.id === tempId ? { ...m, failed: true } : m)),
        })),
      )
      toast(e.message)
    }
  },

  // ---- reactions ----

  // Optimistically toggle the current user's reaction, then PUT/DELETE. The
  // server echo (MESSAGE_REACTION_ADD/REMOVE) is applied idempotently, so it
  // won't double-count our own action.
  toggleReaction: async (channelId, messageId, emoji) => {
    const me = useAuthStore.getState().user
    if (!me || !emoji) return
    const conv = get().messagesByChannel[channelId]
    const msg = conv && conv.items.find((m) => m.id === messageId)
    if (!msg || msg.optimistic) return
    const existing = (msg.reactions || []).find((r) => r.emoji === emoji)
    const hadMe = !!(existing && existing.me)
    const evt = { channel_id: channelId, message_id: messageId, emoji, user_id: me.id }
    if (hadMe) get().applyReactionRemove(evt)
    else get().applyReactionAdd(evt)
    const enc = encodeURIComponent(emoji)
    try {
      if (hadMe) await api.del(`/channels/${channelId}/messages/${messageId}/reactions/${enc}`)
      else await api.put(`/channels/${channelId}/messages/${messageId}/reactions/${enc}`)
    } catch (e) {
      // revert the optimistic change
      if (hadMe) get().applyReactionAdd(evt)
      else get().applyReactionRemove(evt)
      toast(e.message)
    }
  },

  applyReactionAdd: ({ channel_id, message_id, emoji, user_id }) =>
    set((s) => {
      const me = useAuthStore.getState().user
      const isMe = !!(me && user_id === me.id)
      return withConv(s, channel_id, (cur) => ({
        items: cur.items.map((m) => {
          if (m.id !== message_id) return m
          const reactions = (m.reactions || []).slice()
          const idx = reactions.findIndex((r) => r.emoji === emoji)
          if (idx === -1) {
            reactions.push({ emoji, count: 1, me: isMe, users: [user_id] })
          } else {
            const r = reactions[idx]
            if (isMe) {
              if (r.me) return m // already counted ours — idempotent
              reactions[idx] = { ...r, count: r.count + 1, me: true, users: [...(r.users || []), user_id] }
            } else {
              const users = r.users || []
              if (users.includes(user_id)) return m // idempotent
              reactions[idx] = { ...r, count: r.count + 1, me: r.me, users: [...users, user_id] }
            }
          }
          return { ...m, reactions }
        }),
      }))
    }),

  applyReactionRemove: ({ channel_id, message_id, emoji, user_id }) =>
    set((s) => {
      const me = useAuthStore.getState().user
      const isMe = !!(me && user_id === me.id)
      return withConv(s, channel_id, (cur) => ({
        items: cur.items.map((m) => {
          if (m.id !== message_id) return m
          const reactions = (m.reactions || [])
          const idx = reactions.findIndex((r) => r.emoji === emoji)
          if (idx === -1) return m
          const r = reactions[idx]
          const users = r.users || []
          if (isMe) {
            if (!r.me) return m // already removed ours — idempotent
          } else if (users.length && !users.includes(user_id)) {
            return m // idempotent
          }
          const nextCount = r.count - 1
          const next = reactions.slice()
          if (nextCount <= 0) {
            next.splice(idx, 1)
          } else {
            next[idx] = { ...r, count: nextCount, me: isMe ? false : r.me, users: users.filter((u) => u !== user_id) }
          }
          return { ...m, reactions: next }
        }),
      }))
    }),

  editMessage: async (channelId, messageId, content) => {
    const msg = await api.patch(`/channels/${channelId}/messages/${messageId}`, { content })
    get().applyMessageUpdate(msg)
  },

  deleteMessage: async (channelId, messageId) => {
    await api.del(`/channels/${channelId}/messages/${messageId}`)
    get().applyMessageDelete({ id: messageId, channel_id: channelId })
  },

  // ---- guild / channel / invite actions ----

  createGuild: async (name) => {
    const g = await api.post('/guilds', { name })
    get().applyGuildCreate(g)
    await get().selectGuild(g.id)
    return g
  },

  joinGuild: async (code) => {
    const g = await api.post(`/invites/${encodeURIComponent(code)}/join`)
    get().applyGuildCreate(g)
    await get().selectGuild(g.id)
    return g
  },

  createChannel: async (guildId, name, topic, type) => {
    const body = { name }
    if (topic) body.topic = topic
    if (type) body.type = type
    const ch = await api.post(`/guilds/${guildId}/channels`, body)
    get().applyChannelCreate(ch)
    // Only auto-open text channels — voice channels have no message pane and
    // are entered by joining the call, not by selecting them.
    if (get().selectedGuildId === guildId && ch.type !== 'voice') get().selectChannel(ch.id)
    return ch
  },

  createInvite: (guildId) => api.post(`/guilds/${guildId}/invites`),

  // ---- gateway event dispatch (WS §8, payloads per §6) ----

  handleGatewayEvent: (t, d) => {
    switch (t) {
      case 'MESSAGE_CREATE':
        get().applyMessageCreate(d)
        break
      case 'MESSAGE_UPDATE':
        get().applyMessageUpdate(d)
        break
      case 'MESSAGE_DELETE':
        get().applyMessageDelete(d)
        break
      case 'TYPING_START':
        get().applyTypingStart(d)
        break
      case 'PRESENCE_UPDATE':
        get().applyPresenceUpdate(d)
        break
      case 'CHANNEL_CREATE':
        get().applyChannelCreate(d)
        break
      case 'GUILD_CREATE':
        get().applyGuildCreate(d)
        break
      case 'GUILD_MEMBER_ADD':
        get().applyMemberAdd(d)
        break
      case 'MESSAGE_REACTION_ADD':
        get().applyReactionAdd(d)
        break
      case 'MESSAGE_REACTION_REMOVE':
        get().applyReactionRemove(d)
        break
      case 'EFFECT': {
        const me = useAuthStore.getState().user
        // We already played our own effect locally on trigger — skip the echo.
        if (me && d.user_id === me.id) break
        // Only play effects for the guild the user is currently viewing.
        if (d.guild_id && d.guild_id !== get().selectedGuildId) break
        useEffectsStore.getState().playLocal(d.type)
        break
      }
      default:
        break
    }
  },

  applyMessageCreate: (msg) =>
    set((s) => {
      const conv = s.messagesByChannel[msg.channel_id]
      if (!conv) return {}
      if (!conv.loaded) {
        // Initial page still loading: buffer so loadMessages can merge it in
        // rather than losing the event.
        if ((conv.pending || []).some((m) => m.id === msg.id)) return {}
        return {
          messagesByChannel: {
            ...s.messagesByChannel,
            [msg.channel_id]: { ...conv, pending: [...(conv.pending || []), msg] },
          },
        }
      }
      let items
      if (conv.items.some((m) => m.id === msg.id)) {
        items = conv.items.map((m) => (m.id === msg.id ? msg : m))
      } else {
        // If this is the echo of our own optimistic send arriving before the
        // POST response, replace the pending entry instead of duplicating.
        const me = useAuthStore.getState().user
        const pendingIdx =
          me && msg.author.id === me.id
            ? conv.items.findIndex((m) => m.optimistic && !m.failed && m.content === msg.content)
            : -1
        if (pendingIdx !== -1) {
          items = conv.items.slice()
          items[pendingIdx] = msg
        } else {
          items = [...conv.items, msg]
        }
      }
      // A message from someone clears their typing indicator.
      const typing = s.typingByChannel[msg.channel_id]
      let typingByChannel = s.typingByChannel
      if (typing && typing[msg.author.id]) {
        const next = { ...typing }
        delete next[msg.author.id]
        typingByChannel = { ...s.typingByChannel, [msg.channel_id]: next }
      }
      return {
        messagesByChannel: { ...s.messagesByChannel, [msg.channel_id]: { ...conv, items } },
        typingByChannel,
      }
    }),

  applyMessageUpdate: (msg) =>
    set((s) =>
      withConv(s, msg.channel_id, (cur) => ({
        items: cur.items.map((m) => (m.id === msg.id ? msg : m)),
      })),
    ),

  applyMessageDelete: ({ id, channel_id }) =>
    set((s) => withConv(s, channel_id, (cur) => ({ items: cur.items.filter((m) => m.id !== id) }))),

  applyTypingStart: ({ channel_id, user_id, username }) => {
    const me = useAuthStore.getState().user
    if (me && user_id === me.id) return
    set((s) => ({
      typingByChannel: {
        ...s.typingByChannel,
        [channel_id]: {
          ...(s.typingByChannel[channel_id] || {}),
          [user_id]: { username, until: Date.now() + TYPING_TTL_MS },
        },
      },
    }))
  },

  pruneTyping: (channelId) =>
    set((s) => {
      const t = s.typingByChannel[channelId]
      if (!t) return {}
      const now = Date.now()
      const next = {}
      let changed = false
      for (const [uid, v] of Object.entries(t)) {
        if (v.until > now) next[uid] = v
        else changed = true
      }
      if (!changed) return {}
      return { typingByChannel: { ...s.typingByChannel, [channelId]: next } }
    }),

  applyPresenceUpdate: ({ user_id, guild_id, status }) =>
    set((s) => {
      const members = s.membersByGuild[guild_id]
      if (!members || !members.some((m) => m.user_id === user_id)) return {}
      return {
        membersByGuild: {
          ...s.membersByGuild,
          [guild_id]: members.map((m) => (m.user_id === user_id ? { ...m, status } : m)),
        },
      }
    }),

  applyChannelCreate: (ch) =>
    set((s) => {
      const chans = s.channelsByGuild[ch.guild_id]
      if (!chans || chans.some((c) => c.id === ch.id)) return {}
      return { channelsByGuild: { ...s.channelsByGuild, [ch.guild_id]: [...chans, ch] } }
    }),

  applyGuildCreate: (g) =>
    set((s) => (s.guilds.some((x) => x.id === g.id) ? {} : { guilds: [...s.guilds, g], guildsLoaded: true })),

  applyMemberAdd: (member) => {
    // Contract note: the GUILD_MEMBER_ADD `d` payload is a Member object which
    // carries no guild_id. If the gateway includes one we use it; otherwise we
    // conservatively refetch the currently selected guild's member list.
    const guildId = member.guild_id
    if (guildId) {
      set((s) => {
        const members = s.membersByGuild[guildId]
        if (!members || members.some((m) => m.user_id === member.user_id)) return {}
        return { membersByGuild: { ...s.membersByGuild, [guildId]: [...members, member] } }
      })
    } else {
      const selected = get().selectedGuildId
      if (selected) get().loadMembers(selected).catch(() => {})
    }
  },

  // Full state re-sync after a gateway reconnect: anything cached may have
  // missed events, so refetch guilds plus the selected guild/channel.
  resync: async () => {
    try {
      const guilds = await api.get('/users/@me/guilds')
      const gid = get().selectedGuildId
      const cid = get().selectedChannelId
      const next = {
        guilds,
        guildsLoaded: true,
        channelsByGuild: {},
        membersByGuild: {},
        messagesByChannel: {},
        typingByChannel: {},
      }
      if (gid && guilds.some((g) => g.id === gid)) {
        const [channels, members] = await Promise.all([
          api.get(`/guilds/${gid}/channels`),
          api.get(`/guilds/${gid}/members`),
        ])
        next.channelsByGuild = { [gid]: channels }
        next.membersByGuild = { [gid]: members }
        if (cid && channels.some((c) => c.id === cid)) {
          const msgs = await api.get(`/channels/${cid}/messages?limit=${PAGE_SIZE}`)
          next.messagesByChannel = { [cid]: freshConv(msgs) }
        } else {
          next.selectedChannelId = channels[0] ? channels[0].id : null
          if (next.selectedChannelId) {
            const msgs = await api.get(`/channels/${next.selectedChannelId}/messages?limit=${PAGE_SIZE}`)
            next.messagesByChannel = { [next.selectedChannelId]: freshConv(msgs) }
          }
        }
      } else {
        next.selectedGuildId = null
        next.selectedChannelId = null
      }
      set(next)
    } catch {
      // transient — the next reconnect (or user navigation) will retry
    }
  },
}))
