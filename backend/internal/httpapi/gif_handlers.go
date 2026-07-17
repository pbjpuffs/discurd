package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"discurd/internal/models"
)

// Tenor v2 endpoints proxied by the GIF handlers.
const (
	tenorFeaturedURL = "https://tenor.googleapis.com/v2/featured"
	tenorSearchURL   = "https://tenor.googleapis.com/v2/search"
)

// tenorClient bounds every upstream Tenor call; a slow CDN must not pin an API
// worker. Requests also carry the caller's request context for cancellation.
var tenorClient = &http.Client{Timeout: 6 * time.Second}

// gifResponse is the proxied shape returned to clients (§3, FEATURES-v2).
type gifResponse struct {
	Results []models.GifResult `json:"results"`
	Next    string             `json:"next"`
}

// tenorResponse / tenorResult mirror the subset of the Tenor v2 payload we use.
type tenorResponse struct {
	Results []tenorResult `json:"results"`
	Next    string        `json:"next"`
}

type tenorResult struct {
	ID           string                `json:"id"`
	MediaFormats map[string]tenorMedia `json:"media_formats"`
}

type tenorMedia struct {
	URL  string `json:"url"`
	Dims []int  `json:"dims"`
}

func (s *Server) handleGifsTrending(w http.ResponseWriter, r *http.Request) {
	s.proxyTenor(w, r, tenorFeaturedURL, url.Values{})
}

func (s *Server) handleGifsSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, CodeValidationFailed, "q is required")
		return
	}
	v := url.Values{}
	v.Set("q", q)
	s.proxyTenor(w, r, tenorSearchURL, v)
}

// proxyTenor calls a Tenor v2 endpoint with the configured keys and maps the
// response to GifResults. It returns 502 gifs_unavailable when GIFs are not
// configured or the upstream fails, so the client can hide the GIF button.
func (s *Server) proxyTenor(w http.ResponseWriter, r *http.Request, endpoint string, q url.Values) {
	if s.cfg.TenorAPIKey == "" {
		writeError(w, http.StatusBadGateway, CodeGifsUnavailable,
			"GIF search is not configured (set TENOR_API_KEY)")
		return
	}

	q.Set("key", s.cfg.TenorAPIKey)
	q.Set("client_key", s.cfg.TenorClientKey)
	q.Set("limit", "24")
	q.Set("media_filter", "gif,tinygif")
	if pos := r.URL.Query().Get("pos"); pos != "" {
		q.Set("pos", pos)
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, endpoint+"?"+q.Encode(), nil)
	if err != nil {
		s.gifsUnavailable(w, "build tenor request", err)
		return
	}
	resp, err := tenorClient.Do(req)
	if err != nil {
		s.gifsUnavailable(w, "tenor request failed", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		s.gifsUnavailable(w, "tenor upstream status", fmt.Errorf("status %d", resp.StatusCode))
		return
	}

	var tr tenorResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		s.gifsUnavailable(w, "decode tenor response", err)
		return
	}

	out := gifResponse{Results: make([]models.GifResult, 0, len(tr.Results)), Next: tr.Next}
	for _, res := range tr.Results {
		gif := res.MediaFormats["gif"]
		if gif.URL == "" {
			continue
		}
		preview := res.MediaFormats["tinygif"].URL
		if preview == "" {
			preview = gif.URL
		}
		var width, height int
		if len(gif.Dims) == 2 {
			width, height = gif.Dims[0], gif.Dims[1]
		}
		out.Results = append(out.Results, models.GifResult{
			ID:      res.ID,
			URL:     gif.URL,
			Preview: preview,
			Width:   width,
			Height:  height,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// gifsUnavailable logs the upstream cause and returns the 502 the client treats
// as "GIFs disabled".
func (s *Server) gifsUnavailable(w http.ResponseWriter, reason string, err error) {
	s.logger.Warn("gifs unavailable", "reason", reason, "error", err.Error())
	writeError(w, http.StatusBadGateway, CodeGifsUnavailable, "GIF search is temporarily unavailable")
}
