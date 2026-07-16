package obs

import (
	"context"
	"net/http"
	"time"
)

// Healthz is a liveness handler: always 200.
func Healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// ReadyCheck is a named dependency probe.
type ReadyCheck struct {
	Name  string
	Check func(ctx context.Context) error
}

// Readyz returns a readiness handler that runs every check with a short
// timeout and reports 503 with the first failing dependency's name.
func Readyz(checks ...ReadyCheck) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		for _, c := range checks {
			if err := c.Check(ctx); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte("not ready: " + c.Name))
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	}
}
