package ws

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gocql/gocql"
	"github.com/gorilla/websocket"

	"discurd/internal/events"
	"discurd/internal/store"
)

// Protocol constants (docs/ARCHITECTURE.md §8).
const (
	opHello        = "hello"
	opIdentify     = "identify"
	opHeartbeat    = "heartbeat"
	opHeartbeatAck = "heartbeat_ack"
	opDispatch     = "dispatch"

	heartbeatIntervalMS = 30000
	identifyTimeout     = 10 * time.Second
	heartbeatTimeout    = 90 * time.Second

	closeAuthFailed       = 4001
	closeHeartbeatExpired = 4009
	closeGoingAway        = websocket.CloseGoingAway
)

type serverFrame struct {
	Op string `json:"op"`
	T  string `json:"t,omitempty"`
	D  any    `json:"d,omitempty"`
}

type clientFrame struct {
	Op string          `json:"op"`
	D  json.RawMessage `json:"d"`
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	// Auth happens via the identify token; through Traefik traffic is
	// same-origin anyway.
	CheckOrigin: func(*http.Request) bool { return true },
}

// Session is one WebSocket connection.
type Session struct {
	hub  *Hub
	conn *websocket.Conn
	id   string // unique per socket; identifies this session in presence

	sendCh chan []byte
	closed chan struct{} // signals the writer to emit the close frame
	done   chan struct{} // closed after cleanup completes

	closeOnce   sync.Once
	closeCode   int
	closeReason string

	mu         sync.RWMutex
	identified bool
	userID     gocql.UUID
	guildIDs   map[gocql.UUID]struct{}
}

// ServeWS upgrades the request and runs the session until it ends.
func ServeWS(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		hub.logger.Warn("websocket upgrade failed", "error", err.Error())
		return
	}

	s := &Session{
		hub:      hub,
		conn:     conn,
		id:       gocql.TimeUUID().String(),
		sendCh:   make(chan []byte, 256),
		closed:   make(chan struct{}),
		done:     make(chan struct{}),
		guildIDs: map[gocql.UUID]struct{}{},
	}
	if !hub.register(s) { // hub is shutting down
		_ = conn.Close()
		close(s.done)
		return
	}
	hub.metrics.WSConnections.Inc()

	go s.writeLoop()
	s.sendJSON(serverFrame{Op: opHello, D: map[string]int{"heartbeat_interval_ms": heartbeatIntervalMS}})
	s.readLoop()
}

// ---- outbound ----

func (s *Session) sendJSON(f serverFrame) {
	b, err := json.Marshal(f)
	if err != nil {
		s.hub.logger.Error("marshal server frame", "error", err.Error())
		return
	}
	s.send(b)
}

// send enqueues a frame; a full buffer means a dead/slow client, so the
// session is torn down. Returns whether the frame was enqueued.
func (s *Session) send(frame []byte) bool {
	select {
	case <-s.closed:
		return false
	default:
	}
	select {
	case s.sendCh <- frame:
		return true
	default:
		s.close(websocket.ClosePolicyViolation, "client too slow")
		return false
	}
}

func (s *Session) writeLoop() {
	defer s.conn.Close()
	for {
		select {
		case msg := <-s.sendCh:
			_ = s.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := s.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-s.closed:
			_ = s.conn.SetWriteDeadline(time.Now().Add(time.Second))
			_ = s.conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(s.closeCode, s.closeReason))
			return
		}
	}
}

// close records the close code once and signals the writer.
func (s *Session) close(code int, reason string) {
	s.closeOnce.Do(func() {
		s.closeCode = code
		s.closeReason = reason
		close(s.closed)
	})
}

func (s *Session) isClosing() bool {
	select {
	case <-s.closed:
		return true
	default:
		return false
	}
}

// ---- inbound ----

func (s *Session) readLoop() {
	defer s.cleanup()
	s.conn.SetReadLimit(1 << 16)
	_ = s.conn.SetReadDeadline(time.Now().Add(identifyTimeout))

	for {
		_, data, err := s.conn.ReadMessage()
		if err != nil {
			if !s.isClosing() {
				var netErr net.Error
				switch {
				case errors.As(err, &netErr) && netErr.Timeout() && !s.isIdentified():
					s.close(closeAuthFailed, "identify timeout")
				case errors.As(err, &netErr) && netErr.Timeout():
					s.close(closeHeartbeatExpired, "heartbeat timeout")
				default:
					s.close(websocket.CloseNormalClosure, "")
				}
			}
			return
		}

		var f clientFrame
		if err := json.Unmarshal(data, &f); err != nil {
			continue
		}
		switch f.Op {
		case opIdentify:
			s.handleIdentify(f.D)
		case opHeartbeat:
			s.handleHeartbeat()
		}
	}
}

func (s *Session) handleIdentify(raw json.RawMessage) {
	if s.isIdentified() {
		return
	}
	var d struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(raw, &d); err != nil || d.Token == "" {
		s.close(closeAuthFailed, "invalid identify payload")
		return
	}
	sub, err := s.hub.jwt.Verify(d.Token)
	if err != nil {
		s.close(closeAuthFailed, "invalid token")
		return
	}
	uid, err := gocql.ParseUUID(sub)
	if err != nil {
		s.close(closeAuthFailed, "invalid token")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	user, err := s.hub.users.GetByID(ctx, uid)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.close(closeAuthFailed, "unknown user")
		} else {
			s.hub.logger.Error("load user at identify", "error", err.Error())
			s.close(websocket.CloseInternalServerErr, "internal error")
		}
		return
	}
	guildIDs, err := s.hub.guilds.UserGuildIDs(ctx, uid)
	if err != nil {
		s.hub.logger.Error("load guilds at identify", "error", err.Error())
		s.close(websocket.CloseInternalServerErr, "internal error")
		return
	}

	s.mu.Lock()
	s.identified = true
	s.userID = uid
	s.guildIDs = make(map[gocql.UUID]struct{}, len(guildIDs))
	for _, g := range guildIDs {
		s.guildIDs[g] = struct{}{}
	}
	s.mu.Unlock()

	first, err := s.hub.presence.Connect(ctx, uid.String(), s.id)
	if err != nil {
		s.hub.logger.Error("presence connect", "error", err.Error())
	} else if first {
		s.hub.broadcastPresence(ctx, uid, "online")
	}

	ids := make([]string, 0, len(guildIDs))
	for _, g := range guildIDs {
		ids = append(ids, g.String())
	}
	s.sendJSON(serverFrame{Op: opDispatch, T: events.TypeReady, D: map[string]any{
		"user":      user.Model(),
		"guild_ids": ids,
	}})

	_ = s.conn.SetReadDeadline(time.Now().Add(heartbeatTimeout))
}

func (s *Session) handleHeartbeat() {
	s.sendJSON(serverFrame{Op: opHeartbeatAck})
	if !s.isIdentified() {
		return
	}
	_ = s.conn.SetReadDeadline(time.Now().Add(heartbeatTimeout))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.hub.presence.Heartbeat(ctx, s.userIDString(), s.id); err != nil {
		s.hub.logger.Warn("presence heartbeat failed", "error", err.Error())
	}
}

// cleanup runs exactly once when the read loop exits.
func (s *Session) cleanup() {
	s.close(websocket.CloseNormalClosure, "") // no-op if already closing
	s.hub.unregister(s)
	s.hub.metrics.WSConnections.Dec()

	s.mu.RLock()
	identified, uid := s.identified, s.userID
	s.mu.RUnlock()
	if identified {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		last, err := s.hub.presence.Disconnect(ctx, uid.String(), s.id)
		if err != nil {
			s.hub.logger.Error("presence disconnect", "error", err.Error())
		} else if last {
			s.hub.broadcastPresence(ctx, uid, "offline")
		}
	}
	close(s.done)
}

// ---- accessors ----

func (s *Session) isIdentified() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.identified
}

func (s *Session) userIDString() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.identified {
		return ""
	}
	return s.userID.String()
}

func (s *Session) inGuild(guildID gocql.UUID) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.identified {
		return false
	}
	_, ok := s.guildIDs[guildID]
	return ok
}

// addGuild adds a guild id to the session's routing set. Called when the
// user's own GUILD_CREATE is dispatched so subsequent guild-scoped events for
// a just-created/just-joined guild reach this session. Cheap and lock-guarded
// — no I/O — so it is safe to call from the NATS dispatch goroutine.
func (s *Session) addGuild(guildID gocql.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.identified {
		s.guildIDs[guildID] = struct{}{}
	}
}
