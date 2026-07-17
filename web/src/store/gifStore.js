import { create } from 'zustand'
import { api } from '../lib/api'

// Probes GET /gifs/trending once to decide whether the GIF feature is available.
// The backend returns 502 (code gifs_unavailable) when no Tenor key is set — in
// that case the composer hides its GIF button.
let probed = false

export const useGifStore = create((set) => ({
  available: null, // null = unknown, true = usable, false = unavailable (502)

  probe: async () => {
    if (probed) return
    probed = true
    try {
      await api.get('/gifs/trending')
      set({ available: true })
    } catch (e) {
      // Only a definitive 502 (unconfigured) hides the button. Transient errors
      // keep the button so the user can retry from the panel.
      set({ available: e && e.status === 502 ? false : true })
    }
  },
}))
