// Canvas + WebAudio effect engine for the broadcast "effects" system.
// Pure(ish) helpers — no React. Each one-shot effect is an instance with an
// `update(ctx, dt, w, h)` method that draws one frame and returns `true` while
// it is still alive. The overlay owns a single RAF loop and canvas and drives
// every active instance plus the optional always-on storm layer.
//
// Everything is synthesized (bolts, particles, thunder) so there are no asset
// files and nothing violates the app's strict CSP.

export const EFFECT_TYPES = ['lightning', 'confetti', 'hearts', 'snow', 'rain']
export const EFFECT_LABELS = {
  lightning: '⚡ Lightning',
  confetti: '🎉 Confetti',
  hearts: '❤️ Hearts',
  snow: '❄️ Snow',
  rain: '🌧️ Rain',
}
export const EFFECT_EMOJI = {
  lightning: '⚡',
  confetti: '🎉',
  hearts: '❤️',
  snow: '❄️',
  rain: '🌧️',
}

export function prefersReducedMotion() {
  return typeof window !== 'undefined' && window.matchMedia
    ? window.matchMedia('(prefers-reduced-motion: reduce)').matches
    : false
}

const rand = (a, b) => a + Math.random() * (b - a)
const pick = (arr) => arr[(Math.random() * arr.length) | 0]

// ---------------------------------------------------------------- lightning

// A single jagged bolt from the top of the screen towards the bottom, with a
// couple of forks. Regenerated on flicker frames so it feels alive.
function generateBolt(w, h) {
  const points = []
  let x = rand(w * 0.2, w * 0.8)
  let y = 0
  const targetX = x + rand(-w * 0.15, w * 0.15)
  const segments = 14 + ((Math.random() * 8) | 0)
  const step = h / segments
  points.push({ x, y })
  for (let i = 1; i <= segments; i++) {
    y = i * step
    const drift = (targetX - x) * (i / segments)
    x += drift * 0.3 + rand(-w * 0.05, w * 0.05)
    points.push({ x, y })
  }
  // A few forks branching off random mid points.
  const forks = []
  const forkCount = 1 + ((Math.random() * 3) | 0)
  for (let f = 0; f < forkCount; f++) {
    const start = points[2 + ((Math.random() * (points.length - 4)) | 0)]
    const fp = [start]
    let fx = start.x
    let fy = start.y
    const fsegs = 3 + ((Math.random() * 4) | 0)
    for (let i = 0; i < fsegs; i++) {
      fx += rand(-w * 0.08, w * 0.08)
      fy += step * rand(0.4, 0.9)
      fp.push({ x: fx, y: fy })
    }
    forks.push(fp)
  }
  return { points, forks }
}

function strokePath(ctx, pts, width, color, glow) {
  if (pts.length < 2) return
  ctx.beginPath()
  ctx.moveTo(pts[0].x, pts[0].y)
  for (let i = 1; i < pts.length; i++) ctx.lineTo(pts[i].x, pts[i].y)
  ctx.lineWidth = width
  ctx.strokeStyle = color
  ctx.shadowColor = glow
  ctx.shadowBlur = 24
  ctx.lineCap = 'round'
  ctx.lineJoin = 'round'
  ctx.stroke()
}

class LightningEffect {
  constructor(w, h, { reduced }) {
    this.age = 0
    this.reduced = reduced
    this.life = reduced ? 520 : 720
    this.bolts = [generateBolt(w, h)]
    if (Math.random() > 0.5) this.bolts.push(generateBolt(w, h))
    this.nextFlicker = 0
  }

  update(ctx, dt, w, h) {
    this.age += dt

    // White flash that fades out fast.
    const flashDur = this.reduced ? 260 : 200
    const peak = this.reduced ? 0.35 : 0.85
    if (this.age < flashDur) {
      const a = peak * (1 - this.age / flashDur)
      ctx.save()
      ctx.globalAlpha = Math.max(0, a)
      ctx.fillStyle = '#eaf2ff'
      ctx.fillRect(0, 0, w, h)
      ctx.restore()
    }

    // Bolts flicker on/off for the first stretch, regenerating shape.
    const boltWindow = this.reduced ? 200 : 300
    if (this.age < boltWindow) {
      this.nextFlicker -= dt
      if (this.nextFlicker <= 0) {
        this.nextFlicker = rand(30, 70)
        if (Math.random() > 0.35) this.bolts = this.bolts.map(() => generateBolt(w, h))
        this._visible = Math.random() > 0.25
      }
      if (this._visible) {
        ctx.save()
        for (const bolt of this.bolts) {
          strokePath(ctx, bolt.points, 3.5, 'rgba(255,255,255,0.95)', '#9db4ff')
          strokePath(ctx, bolt.points, 1.4, '#ffffff', '#ffffff')
          for (const fork of bolt.forks) strokePath(ctx, fork, 1.6, 'rgba(210,224,255,0.8)', '#9db4ff')
        }
        ctx.restore()
      }
    }
    return this.age < this.life
  }
}

// -------------------------------------------------------------- confetti

class ConfettiEffect {
  constructor(w, h) {
    this.age = 0
    this.life = 3200
    this.colors = ['#5865f2', '#eb459e', '#23a55a', '#f0b232', '#00a8fc', '#ed4245', '#ffffff']
    const count = Math.min(220, Math.floor(w / 6))
    this.parts = []
    for (let i = 0; i < count; i++) {
      this.parts.push({
        x: rand(0, w),
        y: rand(-h * 0.3, 0),
        vx: rand(-0.04, 0.04),
        vy: rand(0.12, 0.34),
        size: rand(5, 11),
        rot: rand(0, Math.PI * 2),
        vr: rand(-0.2, 0.2),
        color: pick(this.colors),
        sway: rand(0.5, 1.6),
      })
    }
  }

  update(ctx, dt, w, h) {
    this.age += dt
    const fade = this.age > this.life - 700 ? Math.max(0, (this.life - this.age) / 700) : 1
    for (const p of this.parts) {
      p.x += (p.vx + Math.sin((this.age / 400) * p.sway) * 0.06) * dt
      p.y += p.vy * dt
      p.rot += p.vr * (dt / 16)
      ctx.save()
      ctx.globalAlpha = fade
      ctx.translate(p.x, p.y)
      ctx.rotate(p.rot)
      ctx.fillStyle = p.color
      ctx.fillRect(-p.size / 2, -p.size / 2, p.size, p.size * 0.6)
      ctx.restore()
    }
    return this.age < this.life
  }
}

// -------------------------------------------------------------- hearts

function heartPath(ctx, x, y, s) {
  ctx.beginPath()
  ctx.moveTo(x, y + s * 0.3)
  ctx.bezierCurveTo(x, y, x - s * 0.5, y, x - s * 0.5, y + s * 0.3)
  ctx.bezierCurveTo(x - s * 0.5, y + s * 0.6, x, y + s * 0.8, x, y + s)
  ctx.bezierCurveTo(x, y + s * 0.8, x + s * 0.5, y + s * 0.6, x + s * 0.5, y + s * 0.3)
  ctx.bezierCurveTo(x + s * 0.5, y, x, y, x, y + s * 0.3)
  ctx.closePath()
  ctx.fill()
}

class HeartsEffect {
  constructor(w, h) {
    this.age = 0
    this.life = 3600
    this.colors = ['#ed4245', '#eb459e', '#ff6b81', '#ff9ff3']
    const count = Math.min(60, Math.floor(w / 20))
    this.parts = []
    for (let i = 0; i < count; i++) {
      this.parts.push({
        x: rand(0, w),
        y: h + rand(0, h * 0.5),
        vy: rand(0.05, 0.13),
        size: rand(16, 34),
        color: pick(this.colors),
        sway: rand(0.6, 1.8),
        phase: rand(0, Math.PI * 2),
        delay: rand(0, 900),
      })
    }
  }

  update(ctx, dt, w, h) {
    this.age += dt
    const fade = this.age > this.life - 800 ? Math.max(0, (this.life - this.age) / 800) : 1
    ctx.save()
    for (const p of this.parts) {
      if (this.age < p.delay) continue
      p.y -= p.vy * dt
      const x = p.x + Math.sin(this.age / 500 + p.phase) * p.sway * 8
      ctx.globalAlpha = fade * 0.9
      ctx.fillStyle = p.color
      heartPath(ctx, x, p.y, p.size)
    }
    ctx.restore()
    return this.age < this.life
  }
}

// -------------------------------------------------------------- snow

class SnowEffect {
  constructor(w, h) {
    this.age = 0
    this.life = 6000
    const count = Math.min(260, Math.floor(w / 5))
    this.parts = []
    for (let i = 0; i < count; i++) {
      this.parts.push({
        x: rand(0, w),
        y: rand(-h, 0),
        r: rand(1.5, 4.5),
        vy: rand(0.03, 0.12),
        sway: rand(0.4, 1.4),
        phase: rand(0, Math.PI * 2),
      })
    }
  }

  update(ctx, dt, w, h) {
    this.age += dt
    const fade = this.age > this.life - 1200 ? Math.max(0, (this.life - this.age) / 1200) : 1
    ctx.save()
    ctx.fillStyle = '#ffffff'
    for (const p of this.parts) {
      p.y += p.vy * dt
      const x = p.x + Math.sin(this.age / 700 + p.phase) * p.sway * 10
      if (p.y > h) p.y = -5
      ctx.globalAlpha = fade * 0.85
      ctx.beginPath()
      ctx.arc(x, p.y, p.r, 0, Math.PI * 2)
      ctx.fill()
    }
    ctx.restore()
    return this.age < this.life
  }
}

// -------------------------------------------------------------- rain

class RainEffect {
  constructor(w, h) {
    this.age = 0
    this.life = 4200
    const count = Math.min(320, Math.floor(w / 4))
    this.parts = []
    for (let i = 0; i < count; i++) {
      this.parts.push({
        x: rand(0, w * 1.2) - w * 0.1,
        y: rand(-h, 0),
        len: rand(10, 26),
        vy: rand(0.7, 1.4),
        slant: rand(1.2, 2.2),
      })
    }
  }

  update(ctx, dt, w, h) {
    this.age += dt
    const fade = this.age > this.life - 900 ? Math.max(0, (this.life - this.age) / 900) : 1
    ctx.save()
    ctx.strokeStyle = 'rgba(150,180,220,0.55)'
    ctx.lineWidth = 1.3
    ctx.globalAlpha = fade
    ctx.beginPath()
    for (const p of this.parts) {
      p.y += p.vy * dt
      p.x += p.slant * (dt / 16)
      if (p.y > h) {
        p.y = -p.len
        p.x = rand(0, w * 1.2) - w * 0.1
      }
      ctx.moveTo(p.x, p.y)
      ctx.lineTo(p.x - p.slant * 4, p.y - p.len)
    }
    ctx.stroke()
    ctx.restore()
    return this.age < this.life
  }
}

export function makeEffect(type, w, h, opts = {}) {
  switch (type) {
    case 'lightning':
      return new LightningEffect(w, h, opts)
    case 'confetti':
      return new ConfettiEffect(w, h, opts)
    case 'hearts':
      return new HeartsEffect(w, h, opts)
    case 'snow':
      return new SnowEffect(w, h, opts)
    case 'rain':
      return new RainEffect(w, h, opts)
    default:
      return null
  }
}

// -------------------------------------------------- ambient storm layer

// A low-opacity, continuous background: faint drifting rain plus the odd dim
// lightning flicker. Deliberately subtle and non-distracting.
export class StormLayer {
  constructor(w, h, { reduced }) {
    this.reduced = reduced
    this.resize(w, h)
    this.flash = 0
    this.nextFlash = rand(2500, 6000)
  }

  resize(w, h) {
    this.w = w
    this.h = h
    const count = reducedCount(this.reduced, Math.min(120, Math.floor(w / 12)))
    this.drops = []
    for (let i = 0; i < count; i++) {
      this.drops.push({
        x: rand(0, w * 1.2) - w * 0.1,
        y: rand(0, h),
        len: rand(8, 18),
        vy: rand(0.35, 0.7),
        slant: rand(0.8, 1.6),
      })
    }
  }

  update(ctx, dt, w, h) {
    if (w !== this.w || h !== this.h) this.resize(w, h)

    // Dim vignette wash so the storm reads even on light content.
    ctx.save()
    ctx.fillStyle = 'rgba(20,24,40,0.05)'
    ctx.fillRect(0, 0, w, h)

    if (!this.reduced) {
      ctx.strokeStyle = 'rgba(140,165,200,0.10)'
      ctx.lineWidth = 1
      ctx.beginPath()
      for (const p of this.drops) {
        p.y += p.vy * dt
        p.x += p.slant * (dt / 16)
        if (p.y > h) {
          p.y = -p.len
          p.x = rand(0, w * 1.2) - w * 0.1
        }
        ctx.moveTo(p.x, p.y)
        ctx.lineTo(p.x - p.slant * 3, p.y - p.len)
      }
      ctx.stroke()
    }

    // Occasional faint flash.
    this.nextFlash -= dt
    if (this.nextFlash <= 0) {
      this.flash = this.reduced ? 0.04 : 0.09
      this.nextFlash = rand(2500, 7000)
    }
    if (this.flash > 0) {
      ctx.globalAlpha = this.flash
      ctx.fillStyle = '#cdd8ff'
      ctx.fillRect(0, 0, w, h)
      this.flash = Math.max(0, this.flash - dt / 260)
    }
    ctx.restore()
    return true // storm runs until explicitly stopped
  }
}

function reducedCount(reduced, n) {
  return reduced ? Math.floor(n / 3) : n
}

// ---------------------------------------------------------------- thunder

// Synthesized thunder: a filtered noise "crack" plus a low sine "rumble" that
// decays over ~1.8s. Returns nothing; best-effort (audio may be blocked before
// a user gesture, which we swallow).
export function playThunder(ctx, reduced) {
  if (!ctx) return
  try {
    const now = ctx.currentTime
    const duration = reduced ? 1.1 : 1.9
    const master = ctx.createGain()
    master.gain.value = reduced ? 0.25 : 0.6
    master.connect(ctx.destination)

    // Noise crack
    const bufferSize = Math.floor(ctx.sampleRate * duration)
    const buffer = ctx.createBuffer(1, bufferSize, ctx.sampleRate)
    const data = buffer.getChannelData(0)
    let last = 0
    for (let i = 0; i < bufferSize; i++) {
      // brown-ish noise for a deeper rumble
      const white = Math.random() * 2 - 1
      last = (last + 0.02 * white) / 1.02
      data[i] = last * 3.5
    }
    const noise = ctx.createBufferSource()
    noise.buffer = buffer

    const lp = ctx.createBiquadFilter()
    lp.type = 'lowpass'
    lp.frequency.setValueAtTime(600, now)
    lp.frequency.exponentialRampToValueAtTime(90, now + duration)

    const nGain = ctx.createGain()
    nGain.gain.setValueAtTime(0.0001, now)
    nGain.gain.exponentialRampToValueAtTime(1, now + 0.04)
    nGain.gain.exponentialRampToValueAtTime(0.3, now + 0.3)
    nGain.gain.exponentialRampToValueAtTime(0.0001, now + duration)

    noise.connect(lp)
    lp.connect(nGain)
    nGain.connect(master)

    // Low rumble oscillator
    const osc = ctx.createOscillator()
    osc.type = 'sine'
    osc.frequency.setValueAtTime(70, now)
    osc.frequency.exponentialRampToValueAtTime(28, now + duration)
    const oGain = ctx.createGain()
    oGain.gain.setValueAtTime(0.35, now)
    oGain.gain.exponentialRampToValueAtTime(0.0001, now + duration)
    osc.connect(oGain)
    oGain.connect(master)

    noise.start(now)
    noise.stop(now + duration)
    osc.start(now)
    osc.stop(now + duration)
  } catch {
    // audio unavailable / blocked — ignore
  }
}
