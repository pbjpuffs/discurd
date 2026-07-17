import { useEffect, useRef } from 'react'
import { useEffectsStore } from '../store/effectsStore'
import { makeEffect, StormLayer, playThunder, prefersReducedMotion } from '../lib/effects'

// Mounted once at the top layer of the app. Owns a single full-screen canvas
// and RAF loop that drives every active one-shot effect plus the optional
// always-on storm layer. Lightning additionally triggers a screen shake and a
// synthesized thunder rumble. Everything is cleaned up on unmount.
export default function EffectsOverlay() {
  const canvasRef = useRef(null)
  const stormMode = useEffectsStore((s) => s.stormMode)
  const lastEffect = useEffectsStore((s) => s.lastEffect)

  const effectsRef = useRef([]) // active one-shot effect instances
  const stormRef = useRef(null) // StormLayer | null
  const rafRef = useRef(0)
  const lastTsRef = useRef(0)
  const sizeRef = useRef({ w: 0, h: 0, dpr: 1 })
  const audioRef = useRef(null)
  const shakeTimerRef = useRef(0)
  const reducedRef = useRef(prefersReducedMotion())
  const frameRef = useRef(null)

  // ---- canvas sizing (device-pixel-ratio aware) ----
  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return undefined
    const resize = () => {
      const dpr = Math.min(window.devicePixelRatio || 1, 2)
      const w = window.innerWidth
      const h = window.innerHeight
      sizeRef.current = { w, h, dpr }
      canvas.width = Math.floor(w * dpr)
      canvas.height = Math.floor(h * dpr)
      canvas.style.width = `${w}px`
      canvas.style.height = `${h}px`
    }
    resize()
    window.addEventListener('resize', resize)
    return () => window.removeEventListener('resize', resize)
  }, [])

  // The RAF frame. Kept in a ref-driven closure so start/stop is idempotent.
  useEffect(() => {
    const frame = (ts) => {
      const canvas = canvasRef.current
      if (!canvas) {
        rafRef.current = 0
        return
      }
      const ctx = canvas.getContext('2d')
      const { w, h, dpr } = sizeRef.current
      const dt = Math.min(50, ts - (lastTsRef.current || ts))
      lastTsRef.current = ts

      ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
      ctx.clearRect(0, 0, w, h)

      if (stormRef.current) stormRef.current.update(ctx, dt, w, h)
      if (effectsRef.current.length) {
        effectsRef.current = effectsRef.current.filter((e) => e.update(ctx, dt, w, h))
      }

      if (effectsRef.current.length || stormRef.current) {
        rafRef.current = requestAnimationFrame(frame)
      } else {
        rafRef.current = 0
      }
    }

    frameRef.current = frame
    return () => {
      if (rafRef.current) cancelAnimationFrame(rafRef.current)
      rafRef.current = 0
    }
  }, [])

  const ensureLoop = () => {
    if (rafRef.current || !frameRef.current) return
    lastTsRef.current = performance.now()
    rafRef.current = requestAnimationFrame(frameRef.current)
  }

  // ---- storm mode on/off ----
  useEffect(() => {
    if (stormMode) {
      const { w, h } = sizeRef.current
      stormRef.current = new StormLayer(w, h, { reduced: reducedRef.current })
      ensureLoop()
    } else {
      stormRef.current = null
    }
  }, [stormMode])

  // ---- one-shot effects ----
  useEffect(() => {
    if (!lastEffect) return
    const { type } = lastEffect
    const { w, h } = sizeRef.current
    const reduced = reducedRef.current

    const inst = makeEffect(type, w, h, { reduced })
    if (inst) {
      effectsRef.current = [...effectsRef.current, inst]
      ensureLoop()
    }

    if (type === 'lightning') {
      // Thunder (best-effort — audio may be blocked without a user gesture).
      try {
        if (!audioRef.current) {
          const AC = window.AudioContext || window.webkitAudioContext
          if (AC) audioRef.current = new AC()
        }
        const ac = audioRef.current
        if (ac) {
          if (ac.state === 'suspended') ac.resume().catch(() => {})
          playThunder(ac, reduced)
        }
      } catch {
        // ignore
      }

      // Screen shake (skipped under reduced motion).
      if (!reduced) {
        const target = document.querySelector('.app-shell')
        if (target) {
          target.classList.add('effect-shake')
          if (shakeTimerRef.current) clearTimeout(shakeTimerRef.current)
          shakeTimerRef.current = window.setTimeout(() => {
            target.classList.remove('effect-shake')
            shakeTimerRef.current = 0
          }, 450)
        }
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [lastEffect])

  // ---- teardown on unmount ----
  useEffect(() => {
    return () => {
      if (rafRef.current) cancelAnimationFrame(rafRef.current)
      rafRef.current = 0
      effectsRef.current = []
      stormRef.current = null
      if (shakeTimerRef.current) clearTimeout(shakeTimerRef.current)
      const target = document.querySelector('.app-shell')
      if (target) target.classList.remove('effect-shake')
      if (audioRef.current) {
        audioRef.current.close().catch(() => {})
        audioRef.current = null
      }
    }
  }, [])

  return <canvas className="effects-overlay" ref={canvasRef} aria-hidden="true" />
}
