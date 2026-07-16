import { create } from 'zustand'
import { Room, RoomEvent, Track } from 'livekit-client'
import { api } from '../lib/api'
import { toast } from './toastStore'

// The LiveKit Room and its live track objects are NOT serializable, so they
// live at module scope — never inside zustand state. The store holds only plain
// descriptors (see buildParticipants) plus a `mediaVersion` counter that ticks
// on every media change so subscribed components re-render and re-run their
// attach/detach effects. Components read the actual MediaStreamTracks through
// the getDisplayTrack / getRemoteAudioTracks getters, which reach into `room`.
let room = null

// Derive the LiveKit signaling URL from the current origin rather than trusting
// the backend `url` field. Traefik proxies `/livekit` to LiveKit's /rtc, so the
// same origin works on localhost, a bare IP, or a domain, over http or https.
function signalUrl() {
  const proto = window.location.protocol === 'https:' ? 'wss' : 'ws'
  return `${proto}://${window.location.host}/livekit`
}

// LiveKit stores our per-user metadata (set by the API when minting the token)
// as a JSON string `{avatar_url}`.
function avatarFromMetadata(metadata) {
  if (!metadata) return ''
  try {
    return JSON.parse(metadata).avatar_url || ''
  } catch {
    return ''
  }
}

function describe(p, isLocal) {
  return {
    identity: p.identity,
    name: p.name || p.identity,
    avatarUrl: avatarFromMetadata(p.metadata),
    isSpeaking: p.isSpeaking,
    micEnabled: p.isMicrophoneEnabled,
    camEnabled: p.isCameraEnabled,
    screenEnabled: p.isScreenShareEnabled,
    isLocal,
  }
}

function buildParticipants() {
  if (!room) return []
  const out = []
  if (room.localParticipant) out.push(describe(room.localParticipant, true))
  for (const p of room.remoteParticipants.values()) out.push(describe(p, false))
  return out
}

function findParticipant(identity) {
  if (!room) return null
  const lp = room.localParticipant
  if (lp && lp.identity === identity) return lp
  for (const p of room.remoteParticipants.values()) {
    if (p.identity === identity) return p
  }
  return null
}

// Re-read the live room and push fresh descriptors into state. `bumpMedia`
// ticks mediaVersion, which makes tiles re-run their track attach/detach
// effects — do that for track add/remove/mute changes, but NOT for frequent
// speaking updates (which only need to repaint the descriptors) or the video
// tiles would flicker every time anyone starts/stops talking.
function syncFromRoom(bumpMedia = true) {
  if (!room) return
  const lp = room.localParticipant
  useVoiceStore.setState((s) => ({
    participants: buildParticipants(),
    localMic: lp ? lp.isMicrophoneEnabled : false,
    localCam: lp ? lp.isCameraEnabled : false,
    localScreen: lp ? lp.isScreenShareEnabled : false,
    mediaVersion: bumpMedia ? s.mediaVersion + 1 : s.mediaVersion,
  }))
}

// Turn a getUserMedia / capture failure into a human message. Insecure origins
// (bare http on an IP) don't expose the camera/mic at all, so call that out.
function mediaErrorMessage(e, what = 'microphone') {
  if (!window.isSecureContext) {
    return `Can't access your ${what}: voice/video capture needs a secure page (https or localhost).`
  }
  const name = e && e.name
  if (name === 'NotAllowedError' || name === 'SecurityError') {
    return `${what[0].toUpperCase()}${what.slice(1)} permission denied. Allow access in your browser to continue.`
  }
  if (name === 'NotFoundError' || name === 'DevicesNotFoundError') {
    return `No ${what} found on this device.`
  }
  if (name === 'NotReadableError') {
    return `Your ${what} is already in use by another application.`
  }
  return (e && e.message) || `Could not start your ${what}.`
}

const RESET = {
  connected: false,
  connecting: false,
  roomChannelId: null,
  roomGuildId: null,
  participants: [],
  localMic: false,
  localCam: false,
  localScreen: false,
  // Whether the main content area should show the call stage. Stays true while
  // in a call until the user navigates to a text channel (Discord-style: you
  // remain connected while browsing). Reset when the call ends.
  viewVoiceStage: false,
}

function wireEvents(r) {
  const onMedia = () => syncFromRoom(true)
  const onSpeaking = () => syncFromRoom(false)
  r.on(RoomEvent.ParticipantConnected, onMedia)
    .on(RoomEvent.ParticipantDisconnected, onMedia)
    .on(RoomEvent.TrackSubscribed, onMedia)
    .on(RoomEvent.TrackUnsubscribed, onMedia)
    .on(RoomEvent.TrackMuted, onMedia)
    .on(RoomEvent.TrackUnmuted, onMedia)
    .on(RoomEvent.ActiveSpeakersChanged, onSpeaking)
    .on(RoomEvent.LocalTrackPublished, onMedia)
    .on(RoomEvent.LocalTrackUnpublished, onMedia)
    .on(RoomEvent.Disconnected, onRoomDisconnected)
}

// Fired for *unexpected* disconnects (server drop, kicked, etc). A manual
// leave() nulls `room` and strips listeners first, so this won't run for that.
function onRoomDisconnected() {
  if (!room) return
  room = null
  useVoiceStore.setState({ ...RESET })
}

export const useVoiceStore = create((set, get) => ({
  ...RESET,
  // Bumped on every media change; components depend on it to re-attach tracks.
  mediaVersion: 0,

  setViewingText: () => set({ viewVoiceStage: false }),
  showStage: () => set({ viewVoiceStage: true }),

  join: async (channelId, guildId) => {
    // Clicking the voice channel always brings the stage into view, even if we
    // are already connected to it or still connecting.
    set({ viewVoiceStage: true })
    const s = get()
    if (s.connecting) return
    if (s.connected && s.roomChannelId === channelId) return

    // Already in another call — tear it down before joining the new one.
    // (leave() clears viewVoiceStage, so we re-assert it just below.)
    if (room || s.connected) await get().leave()

    set({ connecting: true, roomChannelId: channelId, roomGuildId: guildId, viewVoiceStage: true })

    let data
    try {
      data = await api.post(`/channels/${channelId}/voice/token`)
    } catch (e) {
      set({ ...RESET, viewVoiceStage: false })
      toast(e.message)
      return
    }

    const r = new Room({ adaptiveStream: true, dynacast: true })
    room = r
    wireEvents(r)

    try {
      await r.connect(signalUrl(), data.token)
    } catch (e) {
      if (room === r) room = null
      try {
        r.removeAllListeners()
        await r.disconnect()
      } catch {
        // ignore
      }
      set({ ...RESET, viewVoiceStage: false })
      toast('Could not connect to the voice server. Please try again.')
      return
    }

    // The user may have hit leave() (or joined elsewhere) while we were
    // connecting — bail without clobbering the newer room.
    if (room !== r) {
      try {
        r.removeAllListeners()
        await r.disconnect()
      } catch {
        // ignore
      }
      return
    }

    set({ connected: true, connecting: false })
    syncFromRoom()

    // Enable the mic. This is where getUserMedia / secure-context / permission
    // failures surface — stay connected (so you can still hear others) but tell
    // the user why their mic is off.
    try {
      await r.localParticipant.setMicrophoneEnabled(true)
    } catch (e) {
      toast(mediaErrorMessage(e, 'microphone'))
    }
    if (room === r) syncFromRoom()
  },

  leave: async () => {
    const r = room
    room = null
    if (r) {
      try {
        r.removeAllListeners()
        await r.disconnect()
      } catch {
        // already gone
      }
    }
    set({ ...RESET })
  },

  toggleMic: async () => {
    if (!room) return
    const lp = room.localParticipant
    try {
      await lp.setMicrophoneEnabled(!lp.isMicrophoneEnabled)
    } catch (e) {
      toast(mediaErrorMessage(e, 'microphone'))
    }
    syncFromRoom()
  },

  toggleCamera: async () => {
    if (!room) return
    const lp = room.localParticipant
    try {
      await lp.setCameraEnabled(!lp.isCameraEnabled)
    } catch (e) {
      toast(mediaErrorMessage(e, 'camera'))
    }
    syncFromRoom()
  },

  toggleScreenShare: async () => {
    if (!room) return
    const lp = room.localParticipant
    try {
      await lp.setScreenShareEnabled(!lp.isScreenShareEnabled)
    } catch (e) {
      // A user cancelling the screen-picker throws NotAllowedError — that's not
      // worth a scary toast, so swallow the cancel case.
      if (!(e && e.name === 'NotAllowedError')) toast(mediaErrorMessage(e, 'screen'))
    }
    syncFromRoom()
  },

  // ---- imperative getters for the live (non-serializable) track objects ----

  // The video to show for a tile: screen share wins over camera. Returns
  // { track, isScreen } or null when the participant has no active video.
  getDisplayTrack: (identity) => {
    const p = findParticipant(identity)
    if (!p) return null
    const screen = p.getTrackPublication(Track.Source.ScreenShare)
    if (screen && screen.track && !screen.isMuted) return { track: screen.track, isScreen: true }
    const cam = p.getTrackPublication(Track.Source.Camera)
    if (cam && cam.track && !cam.isMuted) return { track: cam.track, isScreen: false }
    return null
  },

  // Every remote audio track (mic + screen-share audio) so the stage can attach
  // them to hidden <audio> elements. Local audio is excluded to avoid echo.
  getRemoteAudioTracks: () => {
    if (!room) return []
    const out = []
    for (const p of room.remoteParticipants.values()) {
      for (const pub of p.trackPublications.values()) {
        if (pub.kind === Track.Kind.Audio && pub.track) {
          out.push({ key: `${p.identity}:${pub.trackSid}`, track: pub.track })
        }
      }
    }
    return out
  },
}))
