import { create } from 'zustand'
import { api } from '../lib/api'

// Fetches + caches public user profiles (GET /users/{id}) and owns the state of
// the single, app-wide profile card popover. Any username/avatar can open the
// card via openProfileFromEvent (below), which anchors it to the click target.
export const useProfileStore = create((set, get) => ({
  profiles: {}, // userId -> profile object
  loading: {}, // userId -> bool
  errored: {}, // userId -> bool
  open: null, // { userId, rect } | null — the currently shown card

  openCard: (userId, rect) => {
    if (!userId) return
    set({ open: { userId, rect } })
    get().fetch(userId)
  },

  closeCard: () => set({ open: null }),

  fetch: async (userId, force = false) => {
    if (!userId) return null
    const st = get()
    if (!force && (st.profiles[userId] || st.loading[userId])) return st.profiles[userId] || null
    set((s) => ({
      loading: { ...s.loading, [userId]: true },
      errored: { ...s.errored, [userId]: false },
    }))
    try {
      const p = await api.get(`/users/${userId}`)
      set((s) => ({ profiles: { ...s.profiles, [userId]: p }, loading: { ...s.loading, [userId]: false } }))
      return p
    } catch {
      set((s) => ({ loading: { ...s.loading, [userId]: false }, errored: { ...s.errored, [userId]: true } }))
      return null
    }
  },

  // Merge a fresh profile into the cache (e.g. after the current user edits it).
  setProfile: (p) => {
    if (!p || !p.id) return
    set((s) => ({ profiles: { ...s.profiles, [p.id]: p } }))
  },
}))

// Open the profile card for `userId`, anchored to the clicked element. Wire this
// onto any clickable username/avatar throughout the app.
export function openProfileFromEvent(e, userId) {
  if (!userId) return
  e.preventDefault()
  e.stopPropagation()
  const r = e.currentTarget.getBoundingClientRect()
  useProfileStore.getState().openCard(userId, { top: r.top, bottom: r.bottom, left: r.left, right: r.right })
}
