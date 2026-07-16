import { useVoiceStore } from '../store/voiceStore'
import { useChatStore } from '../store/chatStore'
import { MicIcon, MicOffIcon, VideoIcon, VideoOffIcon, ScreenShareIcon, PhoneOffIcon } from './Icons'

// Persistent bar shown whenever the user is in a voice call, sitting just above
// the UserPanel. Stays visible while the user browses text channels.
export default function VoiceControlBar() {
  const connected = useVoiceStore((s) => s.connected)
  const localMic = useVoiceStore((s) => s.localMic)
  const localCam = useVoiceStore((s) => s.localCam)
  const localScreen = useVoiceStore((s) => s.localScreen)
  const roomChannelId = useVoiceStore((s) => s.roomChannelId)
  const roomGuildId = useVoiceStore((s) => s.roomGuildId)
  const toggleMic = useVoiceStore((s) => s.toggleMic)
  const toggleCamera = useVoiceStore((s) => s.toggleCamera)
  const toggleScreenShare = useVoiceStore((s) => s.toggleScreenShare)
  const leave = useVoiceStore((s) => s.leave)
  const showStage = useVoiceStore((s) => s.showStage)

  const channel = useChatStore((s) =>
    roomGuildId ? (s.channelsByGuild[roomGuildId] || []).find((c) => c.id === roomChannelId) : null,
  )

  if (!connected) return null

  return (
    <div className="voice-control-bar">
      <button className="voice-status" onClick={showStage} title="Return to call">
        <span className="voice-status-label">
          <span className="voice-status-dot" />
          Voice Connected
        </span>
        <span className="voice-status-channel">{channel ? channel.name : 'Voice'}</span>
      </button>
      <div className="voice-controls">
        <button
          className={`voice-ctl ${localMic ? '' : 'off'}`}
          onClick={toggleMic}
          title={localMic ? 'Mute' : 'Unmute'}
        >
          {localMic ? <MicIcon size={18} /> : <MicOffIcon size={18} />}
        </button>
        <button
          className={`voice-ctl ${localCam ? 'active' : ''}`}
          onClick={toggleCamera}
          title={localCam ? 'Turn off camera' : 'Turn on camera'}
        >
          {localCam ? <VideoIcon size={18} /> : <VideoOffIcon size={18} />}
        </button>
        <button
          className={`voice-ctl ${localScreen ? 'active' : ''}`}
          onClick={toggleScreenShare}
          title={localScreen ? 'Stop sharing' : 'Share your screen'}
        >
          <ScreenShareIcon size={18} />
        </button>
        <button className="voice-ctl hangup" onClick={leave} title="Disconnect">
          <PhoneOffIcon size={18} />
        </button>
      </div>
    </div>
  )
}
