package store

import (
	"context"
	"errors"
	"time"

	"github.com/gocql/gocql"

	"discurd/internal/models"
)

// ChannelRecord is the storage-side channel row.
type ChannelRecord struct {
	ID        gocql.UUID
	GuildID   gocql.UUID
	Name      string
	Type      string
	Topic     string
	CreatedAt time.Time
}

// Model converts the row to the API shape. A null/empty stored type
// normalizes to "text" (§9.5).
func (c ChannelRecord) Model() models.Channel {
	t := c.Type
	if t == "" {
		t = "text"
	}
	return models.Channel{
		ID:        c.ID.String(),
		GuildID:   c.GuildID.String(),
		Name:      c.Name,
		Type:      t,
		Topic:     c.Topic,
		CreatedAt: c.CreatedAt.UTC(),
	}
}

// Channels is the channel repository (channels + channels_by_id).
type Channels struct {
	s *gocql.Session
}

// NewChannels builds the repository.
func NewChannels(s *gocql.Session) *Channels { return &Channels{s: s} }

// Create writes the channel into both the per-guild listing table and the
// by-id lookup table.
func (r *Channels) Create(ctx context.Context, c ChannelRecord) error {
	if err := r.s.Query(
		`INSERT INTO channels (guild_id, channel_id, name, type, topic, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		c.GuildID, c.ID, c.Name, c.Type, c.Topic, c.CreatedAt,
	).WithContext(ctx).Exec(); err != nil {
		return err
	}
	return r.s.Query(
		`INSERT INTO channels_by_id (channel_id, guild_id, name, type, topic, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		c.ID, c.GuildID, c.Name, c.Type, c.Topic, c.CreatedAt,
	).WithContext(ctx).Exec()
}

// Get looks a channel up by id alone (authz path for message endpoints).
func (r *Channels) Get(ctx context.Context, id gocql.UUID) (ChannelRecord, error) {
	var c ChannelRecord
	c.ID = id
	err := r.s.Query(
		`SELECT guild_id, name, type, topic, created_at FROM channels_by_id WHERE channel_id = ?`, id,
	).WithContext(ctx).Scan(&c.GuildID, &c.Name, &c.Type, &c.Topic, &c.CreatedAt)
	if errors.Is(err, gocql.ErrNotFound) {
		return c, ErrNotFound
	}
	return c, err
}

// ListByGuild lists a guild's channels.
func (r *Channels) ListByGuild(ctx context.Context, guildID gocql.UUID) ([]ChannelRecord, error) {
	iter := r.s.Query(
		`SELECT channel_id, name, type, topic, created_at FROM channels WHERE guild_id = ?`, guildID,
	).WithContext(ctx).Iter()
	var out []ChannelRecord
	var c ChannelRecord
	c.GuildID = guildID
	for iter.Scan(&c.ID, &c.Name, &c.Type, &c.Topic, &c.CreatedAt) {
		out = append(out, c)
	}
	return out, iter.Close()
}
