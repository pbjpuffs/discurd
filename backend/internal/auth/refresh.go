package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrRefreshNotFound means the refresh token is unknown, expired, or revoked.
var ErrRefreshNotFound = errors.New("refresh token not found")

// RefreshStore manages opaque refresh tokens in Redis under `refresh:{token}`.
type RefreshStore struct {
	rdb *redis.Client
	ttl time.Duration
}

// NewRefreshStore builds the store with the configured token TTL.
func NewRefreshStore(rdb *redis.Client, ttl time.Duration) *RefreshStore {
	return &RefreshStore{rdb: rdb, ttl: ttl}
}

func refreshKey(token string) string { return "refresh:" + token }

// Issue mints a new opaque token mapped to the user id.
func (s *RefreshStore) Issue(ctx context.Context, userID string) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	token := hex.EncodeToString(buf)
	if err := s.rdb.Set(ctx, refreshKey(token), userID, s.ttl).Err(); err != nil {
		return "", err
	}
	return token, nil
}

// Rotate atomically consumes the old token (GETDEL) and issues a new one.
func (s *RefreshStore) Rotate(ctx context.Context, oldToken string) (userID, newToken string, err error) {
	userID, err = s.rdb.GetDel(ctx, refreshKey(oldToken)).Result()
	if errors.Is(err, redis.Nil) {
		return "", "", ErrRefreshNotFound
	}
	if err != nil {
		return "", "", err
	}
	newToken, err = s.Issue(ctx, userID)
	return userID, newToken, err
}

// Revoke deletes a token (logout).
func (s *RefreshStore) Revoke(ctx context.Context, token string) error {
	return s.rdb.Del(ctx, refreshKey(token)).Err()
}
