// Package store contains the ScyllaDB repositories.
package store

import (
	"context"
	"time"

	"github.com/gocql/gocql"
)

// Connect creates a gocql session against the given hosts/keyspace.
func Connect(hosts []string, keyspace string) (*gocql.Session, error) {
	cluster := gocql.NewCluster(hosts...)
	cluster.Keyspace = keyspace
	cluster.Consistency = gocql.Quorum
	cluster.SerialConsistency = gocql.Serial
	cluster.Timeout = 5 * time.Second
	cluster.ConnectTimeout = 5 * time.Second
	cluster.ProtoVersion = 4
	return cluster.CreateSession()
}

// Ping verifies the session is usable (used by /readyz).
func Ping(ctx context.Context, s *gocql.Session) error {
	return s.Query(`SELECT release_version FROM system.local`).WithContext(ctx).Exec()
}
