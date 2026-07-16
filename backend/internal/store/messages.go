package store

import (
	"context"
	"errors"
	"time"

	"github.com/gocql/gocql"

	"discurd/internal/models"
)

// MessageRecord is the storage-side message row.
type MessageRecord struct {
	ChannelID   gocql.UUID
	Bucket      int
	ID          gocql.UUID // timeuuid
	AuthorID    gocql.UUID
	Content     string
	Attachments []models.Attachment
	EditedAt    *time.Time
}

// CreatedAt derives the creation time from the timeuuid.
func (m MessageRecord) CreatedAt() time.Time {
	return m.ID.Time().UTC()
}

// Messages is the message repository implementing the 10-day bucket pattern.
type Messages struct {
	s *gocql.Session
}

// NewMessages builds the repository.
func NewMessages(s *gocql.Session) *Messages { return &Messages{s: s} }

// Insert writes a message; the bucket is computed from the timeuuid time.
func (r *Messages) Insert(ctx context.Context, m MessageRecord) error {
	return r.s.Query(
		`INSERT INTO messages (channel_id, bucket, message_id, author_id, content, attachments, edited_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		m.ChannelID, m.Bucket, m.ID, m.AuthorID, m.Content, m.Attachments, m.EditedAt,
	).WithContext(ctx).Exec()
}

// Get fetches one message; the bucket is derived from the timeuuid.
func (r *Messages) Get(ctx context.Context, channelID, messageID gocql.UUID) (MessageRecord, error) {
	m := MessageRecord{
		ChannelID: channelID,
		Bucket:    BucketFromTime(messageID.Time()),
		ID:        messageID,
	}
	var edited time.Time
	err := r.s.Query(
		`SELECT author_id, content, attachments, edited_at
		 FROM messages WHERE channel_id = ? AND bucket = ? AND message_id = ?`,
		m.ChannelID, m.Bucket, m.ID,
	).WithContext(ctx).Scan(&m.AuthorID, &m.Content, &m.Attachments, &edited)
	if errors.Is(err, gocql.ErrNotFound) {
		return m, ErrNotFound
	}
	if err != nil {
		return m, err
	}
	if !edited.IsZero() {
		e := edited.UTC()
		m.EditedAt = &e
	}
	return m, nil
}

// UpdateContent edits a message's content and stamps edited_at.
func (r *Messages) UpdateContent(ctx context.Context, m MessageRecord, content string, editedAt time.Time) error {
	return r.s.Query(
		`UPDATE messages SET content = ?, edited_at = ?
		 WHERE channel_id = ? AND bucket = ? AND message_id = ?`,
		content, editedAt, m.ChannelID, m.Bucket, m.ID,
	).WithContext(ctx).Exec()
}

// Delete removes a message.
func (r *Messages) Delete(ctx context.Context, m MessageRecord) error {
	return r.s.Query(
		`DELETE FROM messages WHERE channel_id = ? AND bucket = ? AND message_id = ?`,
		m.ChannelID, m.Bucket, m.ID,
	).WithContext(ctx).Exec()
}

// List returns up to limit messages newest-first. It starts in the bucket of
// `before` (or now), applies `message_id < before` inside that first bucket,
// then walks buckets downward until the limit is filled or the bucket falls
// before bucket(channelCreatedAt).
func (r *Messages) List(ctx context.Context, channelID gocql.UUID, channelCreatedAt time.Time, before *gocql.UUID, limit int) ([]MessageRecord, error) {
	minBucket := BucketFromTime(channelCreatedAt)
	// Start one bucket above "now" so a message written moments after a bucket
	// boundary on a clock-skewed writer node is never skipped. Clamp any
	// client-supplied `before` to this ceiling: a far-future timeuuid must not
	// be able to trigger an unbounded downward walk of empty buckets (one
	// Scylla query each) — the walk is then bounded by the channel's age.
	ceiling := BucketFromTime(time.Now()) + 1
	startBucket := ceiling
	if before != nil {
		if b := BucketFromTime(before.Time()); b < startBucket {
			startBucket = b
		}
	}

	out := make([]MessageRecord, 0, limit)
	for bucket := startBucket; bucket >= minBucket && len(out) < limit; bucket-- {
		remaining := limit - len(out)
		var iter *gocql.Iter
		if before != nil && bucket == startBucket {
			iter = r.s.Query(
				`SELECT message_id, author_id, content, attachments, edited_at
				 FROM messages WHERE channel_id = ? AND bucket = ? AND message_id < ? LIMIT ?`,
				channelID, bucket, *before, remaining,
			).WithContext(ctx).Iter()
		} else {
			iter = r.s.Query(
				`SELECT message_id, author_id, content, attachments, edited_at
				 FROM messages WHERE channel_id = ? AND bucket = ? LIMIT ?`,
				channelID, bucket, remaining,
			).WithContext(ctx).Iter()
		}

		m := MessageRecord{ChannelID: channelID, Bucket: bucket}
		var edited time.Time
		for iter.Scan(&m.ID, &m.AuthorID, &m.Content, &m.Attachments, &edited) {
			m.EditedAt = nil
			if !edited.IsZero() {
				e := edited.UTC()
				m.EditedAt = &e
			}
			out = append(out, m)
			m.Attachments = nil
			edited = time.Time{}
		}
		if err := iter.Close(); err != nil {
			return nil, err
		}
	}
	return out, nil
}
