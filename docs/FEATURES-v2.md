# Discurd v2 features — binding spec

Extends docs/ARCHITECTURE.md. Adds: **user profiles**, **emoji picker**, **message
reactions**, **GIFs (Tenor)**, and a broadcast **effects** system (lightning etc.).
Same conventions as v1 (snake_case JSON, error envelope, guild-scoped WS events).

## 1. Data model (see db/schema.cql — already updated)

- `users` gains `bio text`, `accent_color text`, `pronouns text` (all nullable → treat
  null as empty string in the API).
- New table `reactions`:
  ```
  reactions (channel_id uuid, message_id timeuuid, emoji text, user_id uuid,
             created_at timestamp, PRIMARY KEY ((channel_id, message_id), emoji, user_id))
  ```
  One partition per message; read all reactions for a message with a single partition scan.

## 2. Object shapes (additions)

```jsonc
// User (public profile — no email). @me additionally includes email.
{"id","username","avatar_url","bio":"","accent_color":"","pronouns":"","created_at"}
// Message gains reactions (aggregated for the requesting user):
"reactions":[{"emoji":"🔥","count":3,"me":true}]   // me = did the requester react; [] when none
```

## 3. REST endpoints (all under /api/v1, authenticated unless noted)

| Method & path | Body → Response |
|---|---|
| `GET /users/{user_id}` | → public User (404 if unknown) |
| `PATCH /users/@me` | now also accepts `{username?, bio?, accent_color?, pronouns?}` → User. Validate: bio ≤ 190 chars; pronouns ≤ 40; accent_color must be `#RRGGBB` (6-hex) or empty. |
| `PUT /channels/{channel_id}/messages/{message_id}/reactions/{emoji}` | member → `204`. Adds the requester's reaction. Idempotent. `{emoji}` is URL-encoded; validate 1–32 chars, not whitespace-only. Publishes `MESSAGE_REACTION_ADD`. |
| `DELETE /channels/{channel_id}/messages/{message_id}/reactions/{emoji}` | member → `204`. Removes the requester's reaction. Publishes `MESSAGE_REACTION_REMOVE`. |
| `POST /channels/{channel_id}/effects` | member, `{type}` → `204`. `type` ∈ `lightning|confetti|hearts|snow|rain`. Rate-limited 5/10s per user per channel (Redis limiter, same pattern as messages). Publishes `EFFECT`. |
| `GET /gifs/trending?pos=` | → `{results:[GifResult], next:""}` (proxied from Tenor) |
| `GET /gifs/search?q=&pos=` | → `{results:[GifResult], next:""}` |

`GifResult`: `{"id","url","preview","width","height"}` where `url` is the animated
`.gif`/media URL and `preview` is a small still. If `TENOR_API_KEY` is empty or Tenor
errors, return `502` with error code `gifs_unavailable` and message explaining a key is
needed — the client hides/disables the GIF button gracefully on that.

Reactions hydration: when returning messages (list/create/edit and the WS MESSAGE_CREATE
already carries an empty `reactions:[]`), fetch each message's reactions (one query per
message, may run concurrently) and aggregate to `[{emoji,count,me}]` sorted by first-seen.
`me` uses the requesting user's id. WS-delivered MESSAGE_CREATE/UPDATE include `reactions:[]`
(they're new); the client applies reaction events incrementally.

## 4. WS events (guild-scoped, §6 envelope). Add these `t` constants + gateway routing is unchanged (guild events).

| `t` | `d` |
|---|---|
| `MESSAGE_REACTION_ADD` | `{channel_id, message_id, emoji, user_id}` |
| `MESSAGE_REACTION_REMOVE` | `{channel_id, message_id, emoji, user_id}` |
| `EFFECT` | `{channel_id, guild_id, type, user_id}` |

## 5. Config / env

- `TENOR_API_KEY` (default "") and `TENOR_CLIENT_KEY` (default "discurd") — api service.
- Attachment URL validation: in addition to `/files/attachments/{channel_id}/…`, ALSO allow
  external GIF URLs from these hosts (https only): `media.tenor.com`, `c.tenor.com`,
  `*.tenor.com`, `media.giphy.com`, `*.giphy.com` — but ONLY when `content_type == "image/gif"`.
  This lets a picked GIF be sent as an attachment `{url:<tenor gif>, content_type:"image/gif",
  filename:"gif", size:0}`. Keep the existing rules for everything else.

## 6. Web CSP (web/nginx.conf)

Extend `img-src` to include `https://media.tenor.com https://*.tenor.com https://*.giphy.com`
(GIFs render as `<img>`). Keep everything else as-is.

## 7. Frontend features

**Emoji picker:** use `emoji-mart` (`@emoji-mart/react` + `@emoji-mart/data`, native
unicode — no external images, CSP-safe). One reusable `<EmojiPicker onPick>` used by both
the composer (emoji button inserts into the text) and reactions (add-reaction button).

**User profiles:** every username/avatar (messages, member list, reaction tooltips) is
clickable → a profile popover/card: banner tinted with `accent_color`, big avatar, username,
`pronouns`, online/offline dot, "Member since {created_at}", and `bio`. If it's you, an
"Edit Profile" button opens a modal (bio, pronouns, accent_color color-input, plus the
existing avatar upload). Fetch profiles via `GET /users/{id}` (cache in a store).

**Reactions:** under each message, a row of reaction pills `emoji count`, highlighted when
`me`. Click a pill to toggle (PUT/DELETE). A hover "add reaction" 😀+ button opens the emoji
picker. Hover a pill → tooltip listing who reacted (resolve names from members/profile
cache; fall back to count). Apply `MESSAGE_REACTION_ADD/REMOVE` events incrementally to the
right message.

**GIFs:** a GIF button in the composer opens a panel: trending on open, debounced search box,
responsive grid of `preview` images (swap to `url` on hover), click sends a message with the
GIF as an attachment (per §5). Render `image/gif` attachments inline (animated). Hide the GIF
button if `/gifs/trending` returns 502.

**Effects — make it epic:** a top-layer `<EffectsOverlay>` (fixed, `pointer-events:none`,
above everything) that plays an animation when an `EFFECT` event arrives OR when triggered
locally (play locally immediately AND POST to broadcast to everyone).
- **lightning** (the headline): full-screen white flash, animated jagged bolt(s) drawn on a
  canvas, a brief screen shake, and a synthesized thunder rumble via WebAudio (no asset
  files). Dramatic. This is the always-available one.
- confetti, hearts, snow, rain: canvas particle bursts/ambient.
- Triggers: a **persistent ⚡ Lightning button always visible** (e.g. a floating action
  button, bottom-right, on-screen at all times) + an effects menu (⚡🎉❤️❄️🌧️) in the composer
  toolbar. Keyboard shortcut for lightning too.
- **"Storm Mode"** toggle (persisted in localStorage): an always-on subtle animated storm
  background with occasional faint lightning flickers — ambient, low-opacity, non-distracting,
  runs continuously when enabled.
- Effects are ephemeral (no persistence); everyone in the current guild/channel sees a
  triggered effect via the `EFFECT` event.

Keep the Discord-dark theme, clean up all canvas/audio/listeners on unmount, and respect
`prefers-reduced-motion` (tone effects down / skip shake for those users).
