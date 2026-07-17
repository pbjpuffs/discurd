package store

import (
	"context"
	"time"

	"github.com/gocql/gocql"
)

// ReactionRow is one (emoji, user) pair from a message's reaction partition.
type ReactionRow struct {
	Emoji  string
	UserID gocql.UUID
}

// Reactions is the message-reaction repository. Every reaction for a message
// lives in a single partition ((channel_id, message_id)), so a message's whole
// reaction set is one partition scan.
type Reactions struct {
	s *gocql.Session
}

// NewReactions builds the repository.
func NewReactions(s *gocql.Session) *Reactions { return &Reactions{s: s} }

// Add records a user's reaction. It is idempotent: re-adding the same emoji
// overwrites the same primary key with a fresh timestamp.
func (r *Reactions) Add(ctx context.Context, channelID, messageID gocql.UUID, emoji string, userID gocql.UUID) error {
	return r.s.Query(
		`INSERT INTO reactions (channel_id, message_id, emoji, user_id, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		channelID, messageID, emoji, userID, time.Now().UTC(),
	).WithContext(ctx).Exec()
}

// Remove deletes a user's reaction. Deleting a row that does not exist is a
// no-op, so this is safe to call unconditionally.
func (r *Reactions) Remove(ctx context.Context, channelID, messageID gocql.UUID, emoji string, userID gocql.UUID) error {
	return r.s.Query(
		`DELETE FROM reactions WHERE channel_id = ? AND message_id = ? AND emoji = ? AND user_id = ?`,
		channelID, messageID, emoji, userID,
	).WithContext(ctx).Exec()
}

// ForMessage returns every reaction on a message in a single partition scan.
// Rows come back ordered by the clustering key (emoji, user_id).
func (r *Reactions) ForMessage(ctx context.Context, channelID, messageID gocql.UUID) ([]ReactionRow, error) {
	iter := r.s.Query(
		`SELECT emoji, user_id FROM reactions WHERE channel_id = ? AND message_id = ?`,
		channelID, messageID,
	).WithContext(ctx).Iter()
	var out []ReactionRow
	var row ReactionRow
	for iter.Scan(&row.Emoji, &row.UserID) {
		out = append(out, row)
	}
	return out, iter.Close()
}
