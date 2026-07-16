package store

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"time"

	"github.com/gocql/gocql"

	"discurd/internal/models"
)

const base62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// NewInviteCode returns a cryptographically random 8-char base62 code.
func NewInviteCode() (string, error) {
	buf := make([]byte, 8)
	for i := range buf {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(base62))))
		if err != nil {
			return "", err
		}
		buf[i] = base62[n.Int64()]
	}
	return string(buf), nil
}

// InviteRecord is the storage-side invite row.
type InviteRecord struct {
	Code      string
	GuildID   gocql.UUID
	InviterID gocql.UUID
	CreatedAt time.Time
}

// Model converts the row to the API shape.
func (i InviteRecord) Model() models.Invite {
	return models.Invite{
		Code:      i.Code,
		GuildID:   i.GuildID.String(),
		CreatedAt: i.CreatedAt.UTC(),
	}
}

// Invites is the invite repository.
type Invites struct {
	s *gocql.Session
}

// NewInvites builds the repository.
func NewInvites(s *gocql.Session) *Invites { return &Invites{s: s} }

// Create inserts an invite; LWT guards against the (unlikely) code collision.
func (r *Invites) Create(ctx context.Context, i InviteRecord) error {
	applied, err := r.s.Query(
		`INSERT INTO invites (code, guild_id, inviter_id, created_at) VALUES (?, ?, ?, ?) IF NOT EXISTS`,
		i.Code, i.GuildID, i.InviterID, i.CreatedAt,
	).WithContext(ctx).MapScanCAS(map[string]interface{}{})
	if err != nil {
		return err
	}
	if !applied {
		return ErrConflict
	}
	return nil
}

// Get fetches an invite by code.
func (r *Invites) Get(ctx context.Context, code string) (InviteRecord, error) {
	i := InviteRecord{Code: code}
	err := r.s.Query(
		`SELECT guild_id, inviter_id, created_at FROM invites WHERE code = ?`, code,
	).WithContext(ctx).Scan(&i.GuildID, &i.InviterID, &i.CreatedAt)
	if errors.Is(err, gocql.ErrNotFound) {
		return i, ErrNotFound
	}
	return i, err
}
