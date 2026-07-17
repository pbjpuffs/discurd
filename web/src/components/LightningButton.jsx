import { useEffectsStore } from '../store/effectsStore'
import { BoltIcon } from './Icons'

// Persistent floating action button, always visible while in the app. Triggers
// (and broadcasts) a lightning effect. A keyboard shortcut (Alt+L) does the same
// — registered in AppShell.
export default function LightningButton() {
  const trigger = useEffectsStore((s) => s.trigger)
  return (
    <button
      className="lightning-fab"
      title="Strike lightning (Alt+L)"
      aria-label="Strike lightning"
      onClick={() => trigger('lightning')}
    >
      <BoltIcon size={24} />
    </button>
  )
}
