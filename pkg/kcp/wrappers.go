/*
Copyright 2022 The KCP Authors.

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

package kcp

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/rest"
	k8scache "k8s.io/client-go/tools/cache"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	kcpcache "github.com/kcp-dev/apimachinery/v2/pkg/cache"
	kcpclient "github.com/kcp-dev/apimachinery/v2/pkg/client"
	"github.com/kcp-dev/apimachinery/v2/third_party/informers"
	"github.com/kcp-dev/logicalcluster/v3"
)

// NewClusterAwareManager returns a kcp-aware manager with appropriate defaults for cache and
// client creation.
func NewClusterAwareManager(cfg *rest.Config, options ctrl.Options) (manager.Manager, error) {
	if options.NewCache == nil {
		options.NewCache = NewClusterAwareCache
	}

	if options.NewAPIReader == nil {
		options.NewAPIReader = NewClusterAwareAPIReader
	}

	if options.NewClient == nil {
		options.NewClient = NewClusterAwareClient
	}

	if options.MapperProvider == nil {
		options.MapperProvider = NewClusterAwareMapperProvider
	}

	cfg.Wrap(func(rt http.RoundTripper) http.RoundTripper {
		return newClusterRoundTripper(rt)
	})
	return ctrl.NewManager(cfg, options)
}

// NewClusterAwareCache returns a cache.Cache that handles multi-cluster watches.
func NewClusterAwareCache(config *rest.Config, opts cache.Options) (cache.Cache, error) {
	c := rest.CopyConfig(config)
	c.Host += "/clusters/*"
	opts.NewInformerFunc = informers.NewSharedIndexInformer

	opts.Indexers = k8scache.Indexers{
		kcpcache.ClusterIndexName:             kcpcache.ClusterIndexFunc,
		kcpcache.ClusterAndNamespaceIndexName: kcpcache.ClusterAndNamespaceIndexFunc,
	}
	return cache.New(c, opts)
}

// NewClusterAwareAPIReader returns a client.Reader that provides read-only access to the API server,
// and is configured to use the context to scope requests to the proper cluster. To scope requests,
// pass the request context with the cluster set.
// Example:
//
//	import (
//		"context"
//		kcpclient "github.com/kcp-dev/apimachinery/v2/pkg/client"
//		ctrl "sigs.k8s.io/controller-runtime"
//	)
//	func (r *reconciler)  Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
//		ctx = kcpclient.WithCluster(ctx, req.ObjectKey.Cluster)
//		// from here on pass this context to all client calls
//		...
//	}
func NewClusterAwareAPIReader(config *rest.Config, opts client.Options) (client.Reader, error) {
	httpClient, err := ClusterAwareHTTPClient(config)
	if err != nil {
		return nil, err
	}
	opts.HTTPClient = httpClient
	return cluster.DefaultNewAPIReader(config, opts)
}

// NewClusterAwareClient returns a client.Client that is configured to use the context
// to scope requests to the proper cluster. To scope requests, pass the request context with the cluster set.
// Example:
//
//	import (
//		"context"
//		kcpclient "github.com/kcp-dev/apimachinery/v2/pkg/client"
//		ctrl "sigs.k8s.io/controller-runtime"
//	)
//	func (r *reconciler)  Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
//		ctx = kcpclient.WithCluster(ctx, req.ObjectKey.Cluster)
//		// from here on pass this context to all client calls
//		...
//	}
func NewClusterAwareClient(cache cache.Cache, config *rest.Config, opts client.Options, uncachedObjects ...client.Object) (client.Client, error) {
	httpClient, err := ClusterAwareHTTPClient(config)
	if err != nil {
		return nil, err
	}
	opts.HTTPClient = httpClient
	return cluster.DefaultNewClient(cache, config, opts, uncachedObjects...)
}

// NewClusterAwareClientForConfig returns a client.Client that is configured to use the context to scope
// requests to the proper cluster. To scope requests, pass the request context with the cluster set.
// Example:
//
//	import (
//		"context"
//		kcpclient "github.com/kcp-dev/apimachinery/v2/pkg/client"
//		ctrl "sigs.k8s.io/controller-runtime"
//	)
//	func (r *reconciler)  Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
//		ctx = kcpclient.WithCluster(ctx, req.ObjectKey.Cluster)
//		// from here on pass this context to all client calls
//		...
//	}
func NewClusterAwareClientForConfig(config *rest.Config) (client.Client, error) {
	httpClient, err := ClusterAwareHTTPClient(config)
	if err != nil {
		return nil, err
	}
	restMapper, err := NewClusterAwareMapperProvider(config)
	if err != nil {
		return nil, err
	}
	return client.New(config, client.Options{
		Mapper:     restMapper,
		HTTPClient: httpClient,
	})
}

// ClusterAwareHTTPClient returns an http.Client with a cluster aware round tripper.
func ClusterAwareHTTPClient(config *rest.Config) (*http.Client, error) {
	httpClient, err := rest.HTTPClientFor(config)
	if err != nil {
		return nil, err
	}

	httpClient.Transport = kcpclient.NewClusterRoundTripper(httpClient.Transport)
	return httpClient, nil
}

// NewClusterAwareMapperProvider is a MapperProvider that returns a logical cluster aware meta.RESTMapper.
func NewClusterAwareMapperProvider(c *rest.Config) (meta.RESTMapper, error) {
	mapperCfg := rest.CopyConfig(c)
	if !strings.HasSuffix(mapperCfg.Host, "/clusters/*") {
		mapperCfg.Host += "/clusters/*"
	}

	return apiutil.NewDynamicRESTMapper(mapperCfg)
}

// ClusterAwareBuilderWithOptions returns a cluster aware Cache constructor that will build
// a cache honoring the options argument, this is useful to specify options like
// SelectorsByObject
// WARNING: If SelectorsByObject is specified, filtered out resources are not
// returned.
// WARNING: If UnsafeDisableDeepCopy is enabled, you must DeepCopy any object
// returned from cache get/list before mutating it.
func ClusterAwareBuilderWithOptions(options cache.Options) cache.NewCacheFunc {
	return func(config *rest.Config, opts cache.Options) (cache.Cache, error) {
		if options.Scheme == nil {
			options.Scheme = opts.Scheme
		}
		if options.Mapper == nil {
			options.Mapper = opts.Mapper
		}
		if options.Resync == nil {
			options.Resync = opts.Resync
		}
		if options.Namespace == "" {
			options.Namespace = opts.Namespace
		}
		if opts.Resync == nil {
			opts.Resync = options.Resync
		}

		return NewClusterAwareCache(config, options)
	}
}

// clusterRoundTripper is a cluster aware wrapper around http.RoundTripper.
type clusterRoundTripper struct {
	delegate http.RoundTripper
}

// newClusterRoundTripper creates a new cluster aware round tripper.
func newClusterRoundTripper(delegate http.RoundTripper) *clusterRoundTripper {
	return &clusterRoundTripper{
		delegate: delegate,
	}
}

func (c *clusterRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	cluster, ok := ClusterFromContext(req.Context())
	if ok {
		req = req.Clone(req.Context())
		req.URL.Path = generatePath(req.URL.Path, cluster.Path())
		req.URL.RawPath = generatePath(req.URL.RawPath, cluster.Path())
	}
	return c.delegate.RoundTrip(req)
}

// apiRegex matches any string that has /api/ or /apis/ in it.
var apiRegex = regexp.MustCompile(`(/api/|/apis/)`)

// generatePath formats the request path to target the specified cluster.
func generatePath(originalPath string, clusterPath logicalcluster.Path) string {
	// If the originalPath already has cluster.Path() then the path was already modifed and no change needed
	if strings.Contains(originalPath, clusterPath.RequestPath()) {
		return originalPath
	}
	// If the originalPath has /api/ or /apis/ in it, it might be anywhere in the path, so we use a regex to find and
	// replaces /api/ or /apis/ with $cluster/api/ or $cluster/apis/
	if apiRegex.MatchString(originalPath) {
		return apiRegex.ReplaceAllString(originalPath, fmt.Sprintf("%s$1", clusterPath.RequestPath()))
	}
	// Otherwise, we're just prepending /clusters/$name
	path := clusterPath.RequestPath()
	// if the original path is relative, add a / separator
	if len(originalPath) > 0 && originalPath[0] != '/' {
		path += "/"
	}
	// finally append the original path
	path += originalPath
	return path
}
