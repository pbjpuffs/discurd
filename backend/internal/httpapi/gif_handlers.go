package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"discurd/internal/models"
)

// Upstream GIF endpoints. Giphy is preferred (easy free key at
// developers.giphy.com); Tenor is the fallback (now Google-Cloud-gated).
const (
	giphyTrendingURL = "https://api.giphy.com/v1/gifs/trending"
	giphySearchURL   = "https://api.giphy.com/v1/gifs/search"
	tenorFeaturedURL = "https://tenor.googleapis.com/v2/featured"
	tenorSearchURL   = "https://tenor.googleapis.com/v2/search"
)

// gifClient bounds every upstream call; a slow CDN must not pin an API worker.
// Requests also carry the caller's request context for cancellation.
var gifClient = &http.Client{Timeout: 6 * time.Second}

// gifResponse is the proxied shape returned to clients (§3, FEATURES-v2).
type gifResponse struct {
	Results []models.GifResult `json:"results"`
	Next    string             `json:"next"`
}

func (s *Server) handleGifsTrending(w http.ResponseWriter, r *http.Request) {
	switch {
	case s.cfg.GiphyAPIKey != "":
		s.proxyGiphy(w, r, giphyTrendingURL, url.Values{})
	case s.cfg.TenorAPIKey != "":
		s.proxyTenor(w, r, tenorFeaturedURL, url.Values{})
	default:
		writeError(w, http.StatusBadGateway, CodeGifsUnavailable,
			"GIF search is not configured (set GIPHY_API_KEY or TENOR_API_KEY)")
	}
}

func (s *Server) handleGifsSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, CodeValidationFailed, "q is required")
		return
	}
	switch {
	case s.cfg.GiphyAPIKey != "":
		v := url.Values{}
		v.Set("q", q)
		s.proxyGiphy(w, r, giphySearchURL, v)
	case s.cfg.TenorAPIKey != "":
		v := url.Values{}
		v.Set("q", q)
		s.proxyTenor(w, r, tenorSearchURL, v)
	default:
		writeError(w, http.StatusBadGateway, CodeGifsUnavailable,
			"GIF search is not configured (set GIPHY_API_KEY or TENOR_API_KEY)")
	}
}

// ---------------------------------------------------------------- Giphy

type giphyResponse struct {
	Data       []giphyGif      `json:"data"`
	Pagination giphyPagination `json:"pagination"`
}

type giphyGif struct {
	ID     string                `json:"id"`
	Images map[string]giphyImage `json:"images"`
}

// Giphy reports image dimensions as strings.
type giphyImage struct {
	URL    string `json:"url"`
	Width  string `json:"width"`
	Height string `json:"height"`
}

type giphyPagination struct {
	TotalCount int `json:"total_count"`
	Count      int `json:"count"`
	Offset     int `json:"offset"`
}

// proxyGiphy calls a Giphy v1 endpoint and maps results to GifResults, paging
// via a numeric offset carried in the opaque `pos` cursor.
func (s *Server) proxyGiphy(w http.ResponseWriter, r *http.Request, endpoint string, q url.Values) {
	q.Set("api_key", s.cfg.GiphyAPIKey)
	q.Set("limit", "24")
	q.Set("rating", "pg-13")
	q.Set("bundle", "messaging_non_clips")
	offset := 0
	if pos := r.URL.Query().Get("pos"); pos != "" {
		if n, err := strconv.Atoi(pos); err == nil && n >= 0 {
			offset = n
			q.Set("offset", pos)
		}
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, endpoint+"?"+q.Encode(), nil)
	if err != nil {
		s.gifsUnavailable(w, "build giphy request", err)
		return
	}
	resp, err := gifClient.Do(req)
	if err != nil {
		s.gifsUnavailable(w, "giphy request failed", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		s.gifsUnavailable(w, "giphy upstream status", fmt.Errorf("status %d", resp.StatusCode))
		return
	}

	var gr giphyResponse
	if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
		s.gifsUnavailable(w, "decode giphy response", err)
		return
	}

	out := gifResponse{Results: make([]models.GifResult, 0, len(gr.Data))}
	for _, g := range gr.Data {
		full := firstGiphyImage(g.Images, "downsized", "original", "fixed_height")
		if full.URL == "" {
			continue
		}
		preview := firstGiphyImage(g.Images, "fixed_width_small", "preview_gif", "fixed_width_downsampled", "downsized")
		width, _ := strconv.Atoi(full.Width)
		height, _ := strconv.Atoi(full.Height)
		out.Results = append(out.Results, models.GifResult{
			ID:      g.ID,
			URL:     full.URL,
			Preview: orDefault(preview.URL, full.URL),
			Width:   width,
			Height:  height,
		})
	}
	// Advance the cursor only while more results remain.
	if next := gr.Pagination.Offset + gr.Pagination.Count; next < gr.Pagination.TotalCount {
		out.Next = strconv.Itoa(next)
	} else if offset > 0 && len(gr.Data) == 0 {
		out.Next = ""
	}
	writeJSON(w, http.StatusOK, out)
}

func firstGiphyImage(images map[string]giphyImage, keys ...string) giphyImage {
	for _, k := range keys {
		if img, ok := images[k]; ok && img.URL != "" {
			return img
		}
	}
	return giphyImage{}
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// ---------------------------------------------------------------- Tenor (fallback)

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

// proxyTenor calls a Tenor v2 endpoint with the configured keys and maps the
// response to GifResults.
func (s *Server) proxyTenor(w http.ResponseWriter, r *http.Request, endpoint string, q url.Values) {
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
	resp, err := gifClient.Do(req)
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
