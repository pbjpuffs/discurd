package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"discurd/internal/auth"
	"discurd/internal/config"
	"discurd/internal/events"
	"discurd/internal/objstore"
	"discurd/internal/obs"
	"discurd/internal/presence"
	"discurd/internal/ratelimit"
	"discurd/internal/store"
)

// Server wires the REST handlers to their dependencies.
type Server struct {
	cfg     *config.Config
	logger  *slog.Logger
	metrics *obs.Metrics

	jwt     *auth.JWT
	refresh *auth.RefreshStore
	limiter *ratelimit.Limiter

	users     *store.Users
	guilds    *store.Guilds
	channels  *store.Channels
	messages  *store.Messages
	reactions *store.Reactions
	invites   *store.Invites
	userCache *store.UserCache

	presence  *presence.Tracker
	publisher *events.Publisher
	objects   *objstore.Store

	readyz http.HandlerFunc
}

// Deps carries everything the server needs.
type Deps struct {
	Cfg       *config.Config
	Logger    *slog.Logger
	Metrics   *obs.Metrics
	JWT       *auth.JWT
	Refresh   *auth.RefreshStore
	Limiter   *ratelimit.Limiter
	Users     *store.Users
	Guilds    *store.Guilds
	Channels  *store.Channels
	Messages  *store.Messages
	Reactions *store.Reactions
	Invites   *store.Invites
	UserCache *store.UserCache
	Presence  *presence.Tracker
	Publisher *events.Publisher
	Objects   *objstore.Store
	Readyz    http.HandlerFunc
}

// NewServer builds the server.
func NewServer(d Deps) *Server {
	return &Server{
		cfg:     d.Cfg,
		logger:  d.Logger,
		metrics: d.Metrics,
		jwt:     d.JWT,
		refresh: d.Refresh,
		limiter: d.Limiter,

		users:     d.Users,
		guilds:    d.Guilds,
		channels:  d.Channels,
		messages:  d.Messages,
		reactions: d.Reactions,
		invites:   d.Invites,
		userCache: d.UserCache,

		presence:  d.Presence,
		publisher: d.Publisher,
		objects:   d.Objects,
		readyz:    d.Readyz,
	}
}

// Router assembles the chi mux per the §7 route table.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(s.recoverer, s.observe, s.corsMiddleware)

	r.Get("/healthz", obs.Healthz)
	r.Get("/readyz", s.readyz)
	r.Method(http.MethodGet, "/metrics", s.metrics.Handler())

	r.Route("/api/v1", func(r chi.Router) {
		// Public auth routes (rate limited by client IP).
		r.Group(func(r chi.Router) {
			r.Use(s.globalRateLimit)
			r.Post("/auth/register", s.handleRegister)
			r.Post("/auth/login", s.handleLogin)
			r.Post("/auth/refresh", s.handleRefresh)
		})

		// Authenticated routes (rate limited by user id).
		r.Group(func(r chi.Router) {
			r.Use(s.authMiddleware, s.globalRateLimit)

			r.Post("/auth/logout", s.handleLogout)

			r.Get("/users/@me", s.handleGetMe)
			r.Patch("/users/@me", s.handlePatchMe)
			r.Post("/users/@me/avatar", s.handleUploadAvatar)
			r.Get("/users/@me/guilds", s.handleMyGuilds)
			r.Get("/users/{user_id}", s.handleGetUser)

			r.Get("/gifs/trending", s.handleGifsTrending)
			r.Get("/gifs/search", s.handleGifsSearch)

			r.Post("/guilds", s.handleCreateGuild)
			r.Get("/guilds/{guild_id}", s.handleGetGuild)
			r.Get("/guilds/{guild_id}/channels", s.handleListChannels)
			r.Post("/guilds/{guild_id}/channels", s.handleCreateChannel)
			r.Get("/guilds/{guild_id}/members", s.handleListMembers)
			r.Post("/guilds/{guild_id}/invites", s.handleCreateInvite)

			r.Post("/invites/{code}/join", s.handleJoinInvite)

			r.Get("/channels/{channel_id}/messages", s.handleListMessages)
			r.Post("/channels/{channel_id}/messages", s.handleCreateMessage)
			r.Patch("/channels/{channel_id}/messages/{message_id}", s.handleEditMessage)
			r.Delete("/channels/{channel_id}/messages/{message_id}", s.handleDeleteMessage)
			r.Put("/channels/{channel_id}/messages/{message_id}/reactions/{emoji}", s.handleAddReaction)
			r.Delete("/channels/{channel_id}/messages/{message_id}/reactions/{emoji}", s.handleRemoveReaction)
			r.Post("/channels/{channel_id}/typing", s.handleTyping)
			r.Post("/channels/{channel_id}/effects", s.handleEffect)
			r.Post("/channels/{channel_id}/attachments", s.handleUploadAttachment)
			r.Post("/channels/{channel_id}/voice/token", s.handleVoiceToken)
		})
	})

	return r
}
