package store

import (
	"context"
	"errors"
	"time"

	"github.com/gocql/gocql"

	"discurd/internal/models"
)

// GuildRecord is the storage-side guild row.
type GuildRecord struct {
	ID        gocql.UUID
	Name      string
	OwnerID   gocql.UUID
	IconURL   string
	CreatedAt time.Time
}

// Model converts the row to the API shape.
func (g GuildRecord) Model() models.Guild {
	return models.Guild{
		ID:        g.ID.String(),
		Name:      g.Name,
		OwnerID:   g.OwnerID.String(),
		IconURL:   g.IconURL,
		CreatedAt: g.CreatedAt.UTC(),
	}
}

// MemberRecord is a guild_members row.
type MemberRecord struct {
	GuildID  gocql.UUID
	UserID   gocql.UUID
	JoinedAt time.Time
}

// Guilds is the guild + membership repository.
type Guilds struct {
	s *gocql.Session
}

// NewGuilds builds the repository.
func NewGuilds(s *gocql.Session) *Guilds { return &Guilds{s: s} }

// Create inserts a guild row.
func (r *Guilds) Create(ctx context.Context, g GuildRecord) error {
	return r.s.Query(
		`INSERT INTO guilds (guild_id, name, owner_id, icon_url, created_at) VALUES (?, ?, ?, ?, ?)`,
		g.ID, g.Name, g.OwnerID, g.IconURL, g.CreatedAt,
	).WithContext(ctx).Exec()
}

// Get fetches one guild.
func (r *Guilds) Get(ctx context.Context, id gocql.UUID) (GuildRecord, error) {
	var g GuildRecord
	g.ID = id
	err := r.s.Query(
		`SELECT name, owner_id, icon_url, created_at FROM guilds WHERE guild_id = ?`, id,
	).WithContext(ctx).Scan(&g.Name, &g.OwnerID, &g.IconURL, &g.CreatedAt)
	if errors.Is(err, gocql.ErrNotFound) {
		return g, ErrNotFound
	}
	return g, err
}

// AddMember writes membership in both directions (guild_members + user_guilds).
func (r *Guilds) AddMember(ctx context.Context, guildID, userID gocql.UUID, joinedAt time.Time) error {
	if err := r.s.Query(
		`INSERT INTO guild_members (guild_id, user_id, joined_at) VALUES (?, ?, ?)`,
		guildID, userID, joinedAt,
	).WithContext(ctx).Exec(); err != nil {
		return err
	}
	return r.s.Query(
		`INSERT INTO user_guilds (user_id, guild_id, joined_at) VALUES (?, ?, ?)`,
		userID, guildID, joinedAt,
	).WithContext(ctx).Exec()
}

// IsMember reports whether the user belongs to the guild.
func (r *Guilds) IsMember(ctx context.Context, guildID, userID gocql.UUID) (bool, error) {
	var joined time.Time
	err := r.s.Query(
		`SELECT joined_at FROM guild_members WHERE guild_id = ? AND user_id = ?`, guildID, userID,
	).WithContext(ctx).Scan(&joined)
	if errors.Is(err, gocql.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// Members lists a guild's membership rows.
func (r *Guilds) Members(ctx context.Context, guildID gocql.UUID) ([]MemberRecord, error) {
	iter := r.s.Query(
		`SELECT user_id, joined_at FROM guild_members WHERE guild_id = ?`, guildID,
	).WithContext(ctx).Iter()
	var out []MemberRecord
	var m MemberRecord
	m.GuildID = guildID
	for iter.Scan(&m.UserID, &m.JoinedAt) {
		out = append(out, m)
	}
	return out, iter.Close()
}

// UserGuildIDs lists the guild ids a user belongs to (user_guilds).
func (r *Guilds) UserGuildIDs(ctx context.Context, userID gocql.UUID) ([]gocql.UUID, error) {
	iter := r.s.Query(
		`SELECT guild_id FROM user_guilds WHERE user_id = ?`, userID,
	).WithContext(ctx).Iter()
	var out []gocql.UUID
	var id gocql.UUID
	for iter.Scan(&id) {
		out = append(out, id)
	}
	return out, iter.Close()
}

// GetMany batch-fetches guilds by id with one IN query.
func (r *Guilds) GetMany(ctx context.Context, ids []gocql.UUID) ([]GuildRecord, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	iter := r.s.Query(
		`SELECT guild_id, name, owner_id, icon_url, created_at FROM guilds WHERE guild_id IN ?`, ids,
	).WithContext(ctx).Iter()
	var out []GuildRecord
	var g GuildRecord
	for iter.Scan(&g.ID, &g.Name, &g.OwnerID, &g.IconURL, &g.CreatedAt) {
		out = append(out, g)
	}
	return out, iter.Close()
}
