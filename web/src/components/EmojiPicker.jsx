import { lazy, Suspense } from 'react'

// Reusable dark-themed emoji picker built on emoji-mart (native unicode — no
// external images, CSP-safe). `onPick` receives the native emoji character.
// The emoji-mart bundle is code-split and only fetched when a picker first opens.
// Presentational only: the parent owns open/close + click-outside (matching the
// popover pattern used elsewhere in the app).
const EmojiPickerInner = lazy(() => import('./EmojiPickerInner'))

export default function EmojiPicker({ onPick, className = '' }) {
  return (
    <div className={`emoji-picker-pop ${className}`.trim()}>
      <Suspense fallback={<div className="emoji-picker-loading">Loading emoji…</div>}>
        <EmojiPickerInner onPick={onPick} />
      </Suspense>
    </div>
  )
}
