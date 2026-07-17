import { create } from 'zustand'
import { api } from '../lib/api'
import { useChatStore } from './chatStore'
import { EFFECT_TYPES } from '../lib/effects'

const STORM_KEY = 'discurd_storm_mode'
const VALID = new Set(EFFECT_TYPES)

let nonce = 0

function loadStorm() {
  try {
    return localStorage.getItem(STORM_KEY) === '1'
  } catch {
    return false
  }
}

// Effects are ephemeral. `lastEffect` is a {type, nonce} token that the mounted
// <EffectsOverlay> watches; bumping the nonce fires a fresh play even for the
// same type in quick succession.
export const useEffectsStore = create((set, get) => ({
  lastEffect: null,
  stormMode: loadStorm(),

  // Play an effect on THIS client only (used for incoming EFFECT events).
  playLocal: (type) => {
    if (!VALID.has(type)) return
    set({ lastEffect: { type, nonce: ++nonce } })
  },

  // Play locally immediately AND broadcast to everyone in the current channel.
  trigger: (type) => {
    if (!VALID.has(type)) return
    get().playLocal(type)
    const channelId = useChatStore.getState().selectedChannelId
    if (channelId) {
      api.post(`/channels/${channelId}/effects`, { type }).catch(() => {
        // best-effort broadcast; the local play already happened
      })
    }
  },

  toggleStorm: () =>
    set((s) => {
      const next = !s.stormMode
      try {
        localStorage.setItem(STORM_KEY, next ? '1' : '0')
      } catch {
        // storage unavailable — keep it in memory only
      }
      return { stormMode: next }
    }),
}))
