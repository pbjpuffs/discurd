package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gocql/gocql"

	"discurd/internal/models"
)

// Sentinel errors shared by the repositories.
var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("already exists")
)

// UserRecord is the storage-side user row, including the password hash. The
// nullable profile columns (bio, accent_color, pronouns) scan into empty
// strings when unset.
type UserRecord struct {
	ID           gocql.UUID
	Username     string
	Email        string
	PasswordHash string
	AvatarURL    string
	Bio          string
	AccentColor  string
	Pronouns     string
	CreatedAt    time.Time
}

// Model converts the row to the API shape (email included; callers strip it
// for non-self users).
func (u UserRecord) Model() models.User {
	return models.User{
		ID:          u.ID.String(),
		Username:    u.Username,
		Email:       u.Email,
		AvatarURL:   u.AvatarURL,
		Bio:         u.Bio,
		AccentColor: u.AccentColor,
		Pronouns:    u.Pronouns,
		CreatedAt:   u.CreatedAt.UTC(),
	}
}

// Users is the user repository.
type Users struct {
	s *gocql.Session
}

// NewUsers builds the repository.
func NewUsers(s *gocql.Session) *Users { return &Users{s: s} }

// Create inserts a user, enforcing email and username uniqueness with LWT.
// Returns ErrConflict if either is taken.
func (r *Users) Create(ctx context.Context, u UserRecord) error {
	applied, err := r.s.Query(
		`INSERT INTO users_by_email (email, user_id) VALUES (?, ?) IF NOT EXISTS`,
		u.Email, u.ID,
	).WithContext(ctx).MapScanCAS(map[string]interface{}{})
	if err != nil {
		return fmt.Errorf("reserve email: %w", err)
	}
	if !applied {
		return fmt.Errorf("email %q: %w", u.Email, ErrConflict)
	}

	applied, err = r.s.Query(
		`INSERT INTO users_by_username (username, user_id) VALUES (?, ?) IF NOT EXISTS`,
		u.Username, u.ID,
	).WithContext(ctx).MapScanCAS(map[string]interface{}{})
	if err == nil && !applied {
		err = fmt.Errorf("username %q: %w", u.Username, ErrConflict)
	}
	if err != nil {
		// Roll back the email reservation so the address is not burned.
		_ = r.s.Query(`DELETE FROM users_by_email WHERE email = ? IF EXISTS`, u.Email).
			WithContext(ctx).Exec()
		return err
	}

	if err := r.s.Query(
		`INSERT INTO users (user_id, username, email, password_hash, avatar_url, bio, accent_color, pronouns, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.ID, u.Username, u.Email, u.PasswordHash, u.AvatarURL, u.Bio, u.AccentColor, u.Pronouns, u.CreatedAt,
	).WithContext(ctx).Exec(); err != nil {
		// The final insert failed after both reservations succeeded; release
		// them so the email/username are not permanently burned with no user
		// row behind them. Best-effort — a leaked reservation is recoverable,
		// a wrong error is not.
		_ = r.s.Query(`DELETE FROM users_by_email WHERE email = ? IF EXISTS`, u.Email).WithContext(ctx).Exec()
		_ = r.s.Query(`DELETE FROM users_by_username WHERE username = ? IF EXISTS`, u.Username).WithContext(ctx).Exec()
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

// GetByID fetches one user.
func (r *Users) GetByID(ctx context.Context, id gocql.UUID) (UserRecord, error) {
	var u UserRecord
	u.ID = id
	err := r.s.Query(
		`SELECT username, email, password_hash, avatar_url, bio, accent_color, pronouns, created_at FROM users WHERE user_id = ?`, id,
	).WithContext(ctx).Scan(&u.Username, &u.Email, &u.PasswordHash, &u.AvatarURL, &u.Bio, &u.AccentColor, &u.Pronouns, &u.CreatedAt)
	if errors.Is(err, gocql.ErrNotFound) {
		return u, ErrNotFound
	}
	return u, err
}

// GetByEmail resolves an email (already lowercased) to the full user row.
func (r *Users) GetByEmail(ctx context.Context, email string) (UserRecord, error) {
	var id gocql.UUID
	err := r.s.Query(`SELECT user_id FROM users_by_email WHERE email = ?`, email).
		WithContext(ctx).Scan(&id)
	if errors.Is(err, gocql.ErrNotFound) {
		return UserRecord{}, ErrNotFound
	}
	if err != nil {
		return UserRecord{}, err
	}
	return r.GetByID(ctx, id)
}

// GetByIDs batch-fetches users with a single IN query (author hydration).
func (r *Users) GetByIDs(ctx context.Context, ids []gocql.UUID) (map[gocql.UUID]UserRecord, error) {
	out := make(map[gocql.UUID]UserRecord, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	iter := r.s.Query(
		`SELECT user_id, username, email, password_hash, avatar_url, bio, accent_color, pronouns, created_at
		 FROM users WHERE user_id IN ?`, ids,
	).WithContext(ctx).Iter()
	var u UserRecord
	for iter.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.AvatarURL, &u.Bio, &u.AccentColor, &u.Pronouns, &u.CreatedAt) {
		out[u.ID] = u
	}
	if err := iter.Close(); err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateUsername changes a username, keeping the uniqueness table consistent.
func (r *Users) UpdateUsername(ctx context.Context, id gocql.UUID, oldName, newName string) error {
	applied, err := r.s.Query(
		`INSERT INTO users_by_username (username, user_id) VALUES (?, ?) IF NOT EXISTS`,
		newName, id,
	).WithContext(ctx).MapScanCAS(map[string]interface{}{})
	if err != nil {
		return fmt.Errorf("reserve username: %w", err)
	}
	if !applied {
		return fmt.Errorf("username %q: %w", newName, ErrConflict)
	}
	if err := r.s.Query(`UPDATE users SET username = ? WHERE user_id = ?`, newName, id).
		WithContext(ctx).Exec(); err != nil {
		return err
	}
	return r.s.Query(`DELETE FROM users_by_username WHERE username = ?`, oldName).
		WithContext(ctx).Exec()
}

// UpdateAvatar sets a user's avatar URL.
func (r *Users) UpdateAvatar(ctx context.Context, id gocql.UUID, url string) error {
	return r.s.Query(`UPDATE users SET avatar_url = ? WHERE user_id = ?`, url, id).
		WithContext(ctx).Exec()
}

// UpdateProfile sets a user's profile fields (bio, accent color, pronouns) in a
// single update.
func (r *Users) UpdateProfile(ctx context.Context, id gocql.UUID, bio, accent, pronouns string) error {
	return r.s.Query(
		`UPDATE users SET bio = ?, accent_color = ?, pronouns = ? WHERE user_id = ?`,
		bio, accent, pronouns, id,
	).WithContext(ctx).Exec()
}
