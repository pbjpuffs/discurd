import { useAuthStore } from '../store/authStore'
import { useChatStore } from '../store/chatStore'
import { ensureFreshAccessToken } from './api'

// WebSocket client per ARCHITECTURE.md §8:
//   connect -> receive {op:"hello"} -> send {op:"identify"} -> heartbeat loop
//   -> {op:"dispatch"} frames routed into the chat store.
// Reconnects with exponential backoff (1s -> 30s), refreshing the access
// token first when it has expired, and fully re-identifies each time.

const BACKOFF_MIN_MS = 1000
const BACKOFF_MAX_MS = 30000

let ws = null
let stopped = true
let heartbeatTimer = null
let reconnectTimer = null
let backoff = BACKOFF_MIN_MS
let lastAck = 0
let identifiedOnce = false

export function connectGateway() {
  if (!stopped) return
  stopped = false
  backoff = BACKOFF_MIN_MS
  identifiedOnce = false
  open()
}

export function disconnectGateway() {
  stopped = true
  if (reconnectTimer) {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
  stopHeartbeat()
  if (ws) {
    ws.onclose = null
    ws.onmessage = null
    ws.onerror = null
    try {
      ws.close(1000)
    } catch {
      // already closed
    }
    ws = null
  }
  useChatStore.getState().setWsConnected(false)
}

function stopHeartbeat() {
  if (heartbeatTimer) {
    clearInterval(heartbeatTimer)
    heartbeatTimer = null
  }
}

async function open() {
  if (stopped) return
  let token
  try {
    token = await ensureFreshAccessToken()
  } catch {
    if (!useAuthStore.getState().refreshToken) {
      // Logged out / session dead — the auth store was cleared and the router
      // will send the user to /login. Stop trying.
      stopped = true
      return
    }
    scheduleReconnect() // transient (e.g. network) failure
    return
  }
  if (stopped) return

  const proto = window.location.protocol === 'https:' ? 'wss' : 'ws'
  const socket = new WebSocket(`${proto}://${window.location.host}/ws`)
  ws = socket
  socket.onmessage = (ev) => {
    let frame
    try {
      frame = JSON.parse(ev.data)
    } catch {
      return
    }
    handleFrame(socket, frame, token)
  }
  socket.onerror = () => {
    // the close event follows and drives reconnection
  }
  socket.onclose = () => {
    if (socket !== ws) return
    ws = null
    scheduleReconnect()
  }
}

function handleFrame(socket, frame, token) {
  switch (frame.op) {
    case 'hello': {
      socket.send(JSON.stringify({ op: 'identify', d: { token } }))
      lastAck = Date.now()
      const interval = (frame.d && frame.d.heartbeat_interval_ms) || 30000
      stopHeartbeat()
      heartbeatTimer = setInterval(() => {
        if (socket.readyState !== WebSocket.OPEN) return
        if (Date.now() - lastAck > interval * 3) {
          // Server stopped acking — treat the socket as dead and reconnect.
          try {
            socket.close()
          } catch {
            // ignore
          }
          return
        }
        socket.send(JSON.stringify({ op: 'heartbeat' }))
      }, interval)
      break
    }
    case 'heartbeat_ack':
      lastAck = Date.now()
      break
    case 'dispatch': {
      const chat = useChatStore.getState()
      if (frame.t === 'READY') {
        backoff = BACKOFF_MIN_MS
        chat.setWsConnected(true)
        if (frame.d && frame.d.user) {
          const auth = useAuthStore.getState()
          auth.setUser({ ...auth.user, ...frame.d.user })
        }
        if (identifiedOnce) chat.resync() // reconnected: caches may be stale
        identifiedOnce = true
      } else {
        chat.handleGatewayEvent(frame.t, frame.d)
      }
      break
    }
    default:
      break
  }
}

function scheduleReconnect() {
  stopHeartbeat()
  useChatStore.getState().setWsConnected(false)
  if (stopped || reconnectTimer) return
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null
    open()
  }, backoff)
  backoff = Math.min(backoff * 2, BACKOFF_MAX_MS)
}
