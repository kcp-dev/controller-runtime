package kcp

import (
	"context"

	"github.com/kcp-dev/logicalcluster/v3"
)

type key int

const (
	keyCluster key = iota
)

// WithCluster injects a cluster name into a context.
func WithCluster(ctx context.Context, cluster logicalcluster.Name) context.Context {
	return context.WithValue(ctx, keyCluster, cluster)
}

// ClusterFromContext extracts a cluster name from the context.
func ClusterFromContext(ctx context.Context) (logicalcluster.Name, bool) {
	s, ok := ctx.Value(keyCluster).(logicalcluster.Name)
	return s, ok
}
