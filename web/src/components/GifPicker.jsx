import { useEffect, useRef, useState } from 'react'
import { api } from '../lib/api'

// GIF panel: loads trending on open, debounced search, responsive masonry of
// still previews that swap to the animated URL on hover. Clicking a GIF calls
// onPick(gif). Uses the backend /gifs proxy (Tenor).
export default function GifPicker({ onPick }) {
  const [query, setQuery] = useState('')
  const [results, setResults] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const inputRef = useRef(null)

  useEffect(() => {
    if (inputRef.current) inputRef.current.focus()
  }, [])

  useEffect(() => {
    let cancelled = false
    const q = query.trim()
    setLoading(true)
    setError(false)
    const path = q ? `/gifs/search?q=${encodeURIComponent(q)}` : '/gifs/trending'
    // Debounce searches; load trending immediately.
    const delay = q ? 350 : 0
    const timer = setTimeout(() => {
      api
        .get(path)
        .then((res) => {
          if (cancelled) return
          setResults((res && res.results) || [])
          setLoading(false)
        })
        .catch(() => {
          if (cancelled) return
          setError(true)
          setResults([])
          setLoading(false)
        })
    }, delay)
    return () => {
      cancelled = true
      clearTimeout(timer)
    }
  }, [query])

  return (
    <div className="gif-picker-pop" onMouseDown={(e) => e.stopPropagation()}>
      <div className="gif-search">
        <input
          ref={inputRef}
          type="text"
          placeholder="Search Tenor"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
      </div>
      <div className="gif-grid">
        {loading && <div className="gif-status">Loading…</div>}
        {!loading && error && <div className="gif-status">Couldn’t load GIFs. Try again.</div>}
        {!loading && !error && results.length === 0 && <div className="gif-status">No results.</div>}
        {!loading &&
          !error &&
          results.map((g) => (
            <button
              key={g.id}
              className="gif-cell"
              title="Send this GIF"
              onClick={() => onPick(g)}
              style={g.width && g.height ? { aspectRatio: `${g.width} / ${g.height}` } : undefined}
            >
              <img
                src={g.preview}
                alt=""
                loading="lazy"
                onMouseEnter={(e) => {
                  e.currentTarget.src = g.url
                }}
                onMouseLeave={(e) => {
                  e.currentTarget.src = g.preview
                }}
              />
            </button>
          ))}
      </div>
    </div>
  )
}
