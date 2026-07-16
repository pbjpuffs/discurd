// Package objstore wraps the MinIO client for avatar/attachment uploads
// (docs/ARCHITECTURE.md §9).
package objstore

import (
	"context"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Bucket names are contract; minio-init creates them with anonymous download.
const (
	BucketAvatars     = "avatars"
	BucketAttachments = "attachments"
)

// Store is the object storage client.
type Store struct {
	client *minio.Client
}

// New builds a client (does not dial; use Ping to verify reachability).
func New(endpoint, accessKey, secretKey string, useSSL bool) (*Store, error) {
	c, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, err
	}
	return &Store{client: c}, nil
}

// Ping verifies MinIO is reachable and the avatars bucket exists.
func (s *Store) Ping(ctx context.Context) error {
	_, err := s.client.BucketExists(ctx, BucketAvatars)
	return err
}

// Upload stores an object and returns its relative public URL
// (`/files/{bucket}/{key}` — Traefik strips /files and proxies to MinIO).
func (s *Store) Upload(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) (string, error) {
	_, err := s.client.PutObject(ctx, bucket, key, r, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", err
	}
	return "/files/" + bucket + "/" + key, nil
}
