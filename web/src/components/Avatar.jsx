import { avatarColor } from '../lib/format'

export default function Avatar({ name, url, size = 32 }) {
  if (url) {
    return <img className="avatar" src={url} alt={name || ''} style={{ width: size, height: size }} />
  }
  return (
    <div
      className="avatar avatar-letter"
      style={{ width: size, height: size, background: avatarColor(name), fontSize: Math.round(size * 0.42) }}
    >
      {(name || '?').charAt(0).toUpperCase()}
    </div>
  )
}
