/*
Copyright 2024 The KCP Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"

	"github.com/kcp-dev/logicalcluster/v3"
	"sigs.k8s.io/controller-runtime/pkg/kontext"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// WithClusterInContext injects a cluster name into a context such that
// cluster clients and cache work out of the box.
func WithClusterInContext(r reconcile.Reconciler) reconcile.Reconciler {
	return reconcile.Func(func(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
		ctx = kontext.WithCluster(ctx, logicalcluster.Name(req.ClusterName))
		return r.Reconcile(ctx, req)
	})
}
