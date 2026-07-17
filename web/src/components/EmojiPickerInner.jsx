import data from '@emoji-mart/data'
import Picker from '@emoji-mart/react'

// The actual emoji-mart picker. Loaded lazily (see EmojiPicker.jsx) so the heavy
// emoji dataset stays out of the initial bundle.
export default function EmojiPickerInner({ onPick }) {
  return (
    <Picker
      data={data}
      set="native"
      theme="dark"
      onEmojiSelect={(emoji) => onPick && onPick(emoji.native)}
      previewPosition="none"
      skinTonePosition="search"
      navPosition="top"
      perLine={8}
      emojiButtonSize={32}
      emojiSize={20}
      maxFrequentRows={2}
    />
  )
}
