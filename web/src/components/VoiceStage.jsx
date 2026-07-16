import { useEffect, useRef, useState } from 'react'
import { useVoiceStore } from '../store/voiceStore'
import { useChatStore } from '../store/chatStore'
import Avatar from './Avatar'
import { VolumeIcon, MicOffIcon } from './Icons'

export default function VoiceStage() {
  const connecting = useVoiceStore((s) => s.connecting)
  const participants = useVoiceStore((s) => s.participants)
  const roomChannelId = useVoiceStore((s) => s.roomChannelId)
  const roomGuildId = useVoiceStore((s) => s.roomGuildId)
  const channel = useChatStore((s) =>
    roomGuildId ? (s.channelsByGuild[roomGuildId] || []).find((c) => c.id === roomChannelId) : null,
  )

  return (
    <main className="voice-stage">
      <header className="channel-header">
        <span className="channel-header-hash">
          <VolumeIcon size={22} />
        </span>
        <span className="channel-title">{channel ? channel.name : 'Voice'}</span>
        <span className="header-divider" />
        <span className="voice-connected-label">Voice Connected</span>
      </header>
      <div className="voice-stage-body">
        {connecting && participants.length === 0 ? (
          <div className="voice-stage-connecting">Connecting to voice…</div>
        ) : (
          <div className="voice-grid" data-count={participants.length}>
            {participants.map((p) => (
              <ParticipantTile key={p.identity} p={p} />
            ))}
          </div>
        )}
      </div>
      <RemoteAudio />
    </main>
  )
}

function ParticipantTile({ p }) {
  const mediaVersion = useVoiceStore((s) => s.mediaVersion)
  const getDisplayTrack = useVoiceStore((s) => s.getDisplayTrack)
  const videoRef = useRef(null)
  const [videoState, setVideoState] = useState({ active: false, isScreen: false })

  // Attach the LiveKit video track (screen share or camera) to the <video>
  // element, and detach on cleanup so switching tracks / unmounting never
  // leaks a MediaStream. Re-runs whenever media changes (mediaVersion tick).
  useEffect(() => {
    const el = videoRef.current
    if (!el) return
    const info = getDisplayTrack(p.identity)
    if (info && info.track) {
      info.track.attach(el)
      setVideoState({ active: true, isScreen: info.isScreen })
      return () => {
        try {
          info.track.detach(el)
        } catch {
          // element/track already gone
        }
      }
    }
    setVideoState({ active: false, isScreen: false })
    return undefined
  }, [p.identity, mediaVersion, getDisplayTrack])

  return (
    <div className={`voice-tile ${p.isSpeaking && p.micEnabled ? 'speaking' : ''}`}>
      {/* Always render the element so the attach effect has a stable ref; hide
          it when there is no video. Muted on every tile — audio is played by the
          dedicated <audio> sinks, avoiding double playback and local echo. */}
      <video
        ref={videoRef}
        className={`voice-video ${videoState.active ? '' : 'hidden'} ${videoState.isScreen ? 'contain' : ''}`}
        autoPlay
        playsInline
        muted
      />
      {!videoState.active && (
        <div className="voice-tile-avatar">
          <Avatar name={p.name} url={p.avatarUrl} size={80} />
        </div>
      )}
      <div className="voice-tile-overlay">
        {!p.micEnabled && (
          <span className="voice-tile-muted" title="Muted">
            <MicOffIcon size={16} />
          </span>
        )}
        <span className="voice-tile-name">
          {p.name}
          {p.isLocal ? ' (you)' : ''}
        </span>
      </div>
    </div>
  )
}

// Hidden container that plays every remote audio track. Keyed by a stable
// identity:trackSid so a sink mounts/unmounts exactly with its track.
function RemoteAudio() {
  const mediaVersion = useVoiceStore((s) => s.mediaVersion)
  const getRemoteAudioTracks = useVoiceStore((s) => s.getRemoteAudioTracks)
  // Recomputed on every mediaVersion change (this component re-renders because
  // it subscribes to mediaVersion above).
  void mediaVersion
  const tracks = getRemoteAudioTracks()
  return (
    <div className="voice-audio-sinks" aria-hidden="true">
      {tracks.map((t) => (
        <AudioSink key={t.key} track={t.track} />
      ))}
    </div>
  )
}

function AudioSink({ track }) {
  const ref = useRef(null)
  useEffect(() => {
    const el = ref.current
    if (!el || !track) return undefined
    track.attach(el)
    return () => {
      try {
        track.detach(el)
      } catch {
        // already detached
      }
    }
  }, [track])
  return <audio ref={ref} autoPlay />
}
