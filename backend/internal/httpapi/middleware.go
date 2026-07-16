package httpapi

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gocql/gocql"
)

type ctxKey int

const userIDKey ctxKey = 1

// userIDFrom returns the authenticated user id from the request context.
func userIDFrom(ctx context.Context) (gocql.UUID, bool) {
	id, ok := ctx.Value(userIDKey).(gocql.UUID)
	return id, ok
}

// authMiddleware validates the Bearer access token and stores the user id.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		token, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || token == "" {
			writeError(w, http.StatusUnauthorized, CodeUnauthorized, "missing bearer token")
			return
		}
		sub, err := s.jwt.Verify(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, CodeUnauthorized, "invalid or expired token")
			return
		}
		uid, err := gocql.ParseUUID(sub)
		if err != nil {
			writeError(w, http.StatusUnauthorized, CodeUnauthorized, "invalid token subject")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userIDKey, uid)))
	})
}

// globalRateLimit enforces 120 req/60s keyed by user id when authenticated,
// client IP otherwise.
func (s *Server) globalRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := clientIP(r)
		if uid, ok := userIDFrom(r.Context()); ok {
			id = uid.String()
		}
		allowed, err := s.limiter.Allow(r.Context(), "global", id, 120, 60*time.Second)
		if err != nil {
			s.logger.Warn("rate limiter unavailable, failing open", "error", err.Error())
		}
		if !allowed {
			writeError(w, http.StatusTooManyRequests, CodeRateLimited, "too many requests")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	// Behind Traefik the client address arrives in X-Forwarded-For.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if first, _, ok := strings.Cut(xff, ","); ok {
			return strings.TrimSpace(first)
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// corsMiddleware honors CORS_ORIGINS and short-circuits preflights.
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	allowed := make(map[string]bool, len(s.cfg.CORSOrigins))
	allowAll := false
	for _, o := range s.cfg.CORSOrigins {
		if o == "*" {
			allowAll = true
		}
		allowed[o] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && (allowAll || allowed[origin]) {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Vary", "Origin")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			h.Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.status = code
	sr.ResponseWriter.WriteHeader(code)
}

// observe records request logs and the §11 HTTP metrics.
func (s *Server) observe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		route := "unmatched"
		if rctx := chi.RouteContext(r.Context()); rctx != nil {
			if p := rctx.RoutePattern(); p != "" {
				route = p
			}
		}
		elapsed := time.Since(start)
		s.metrics.HTTPRequests.WithLabelValues(
			s.cfg.ServiceName, r.Method, route, strconv.Itoa(rec.status)).Inc()
		s.metrics.HTTPDuration.WithLabelValues(s.cfg.ServiceName, route).
			Observe(elapsed.Seconds())
		s.logger.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"route", route,
			"status", rec.status,
			"duration_ms", elapsed.Milliseconds(),
			"remote", clientIP(r),
		)
	})
}

// recoverer converts panics into logged 500s.
func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.logger.Error("panic in handler", "panic", rec, "path", r.URL.Path)
				writeError(w, http.StatusInternalServerError, CodeInternal, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
