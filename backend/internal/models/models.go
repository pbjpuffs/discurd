// Package models defines the domain structs and their JSON wire shapes
// per docs/ARCHITECTURE.md §7.
package models

import "time"

// User is the API user object. Email is serialized only when set — handlers
// blank it for anyone other than the requesting user.
type User struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	Email       string    `json:"email,omitempty"`
	AvatarURL   string    `json:"avatar_url"`
	Bio         string    `json:"bio"`
	AccentColor string    `json:"accent_color"`
	Pronouns    string    `json:"pronouns"`
	CreatedAt   time.Time `json:"created_at"`
}

// Public returns a copy without the email field.
func (u User) Public() User {
	u.Email = ""
	return u
}

// Guild is the API guild object.
type Guild struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	OwnerID   string    `json:"owner_id"`
	IconURL   string    `json:"icon_url"`
	CreatedAt time.Time `json:"created_at"`
}

// Channel is the API channel object. Type is "text" | "voice" (default "text").
type Channel struct {
	ID        string    `json:"id"`
	GuildID   string    `json:"guild_id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Topic     string    `json:"topic"`
	CreatedAt time.Time `json:"created_at"`
}

// Member is a guild member with hydrated user fields and live presence.
type Member struct {
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	AvatarURL string    `json:"avatar_url"`
	JoinedAt  time.Time `json:"joined_at"`
	Status    string    `json:"status"` // "online" | "offline"
	IsOwner   bool      `json:"is_owner"`
}

// Attachment is an uploaded file reference. The cql tags map it onto the
// frozen<attachment> UDT in ScyllaDB.
type Attachment struct {
	URL         string `json:"url" cql:"url"`
	Filename    string `json:"filename" cql:"filename"`
	Size        int64  `json:"size" cql:"size"`
	ContentType string `json:"content_type" cql:"content_type"`
}

// MessageAuthor is the compact author shape embedded in messages.
type MessageAuthor struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
}

// Reaction is an aggregated emoji reaction on a message. Me reports whether the
// requesting user is among those who reacted with this emoji.
type Reaction struct {
	Emoji string `json:"emoji"`
	Count int    `json:"count"`
	Me    bool   `json:"me"`
}

// Message is the API message object. Author is hydrated by the API layer.
type Message struct {
	ID          string        `json:"id"` // timeuuid
	ChannelID   string        `json:"channel_id"`
	GuildID     string        `json:"guild_id"`
	Author      MessageAuthor `json:"author"`
	Content     string        `json:"content"`
	Attachments []Attachment  `json:"attachments"`
	Reactions   []Reaction    `json:"reactions"` // always non-nil; [] when none
	CreatedAt   time.Time     `json:"created_at"`
	EditedAt    *time.Time    `json:"edited_at"` // null when never edited
}

// GifResult is a single GIF returned by the Tenor proxy (docs/FEATURES-v2.md §3).
type GifResult struct {
	ID      string `json:"id"`
	URL     string `json:"url"`
	Preview string `json:"preview"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
}

// Invite is the API invite object.
type Invite struct {
	Code      string    `json:"code"`
	GuildID   string    `json:"guild_id"`
	CreatedAt time.Time `json:"created_at"`
}
