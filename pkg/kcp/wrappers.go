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
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	k8scache "k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/kontext"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	kcpcache "github.com/kcp-dev/apimachinery/v2/pkg/cache"
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
		options.MapperProvider = newWildcardClusterMapperProvider
	}

	cfg.Wrap(func(rt http.RoundTripper) http.RoundTripper {
		return newClusterAwareRoundTripper(rt)
	})
	return ctrl.NewManager(cfg, options)
}

// NewInformerWithClusterIndexes returns a SharedIndexInformer that is configured
// ClusterIndexName and ClusterAndNamespaceIndexName indexes.
func NewInformerWithClusterIndexes(lw k8scache.ListerWatcher, obj runtime.Object, syncPeriod time.Duration, indexers k8scache.Indexers) k8scache.SharedIndexInformer {
	indexers[kcpcache.ClusterIndexName] = kcpcache.ClusterIndexFunc
	indexers[kcpcache.ClusterAndNamespaceIndexName] = kcpcache.ClusterAndNamespaceIndexFunc

	return informers.NewSharedIndexInformer(lw, obj, syncPeriod, indexers)
}

// NewClusterAwareCache returns a cache.Cache that handles multi-cluster watches.
func NewClusterAwareCache(config *rest.Config, opts cache.Options) (cache.Cache, error) {
	c := rest.CopyConfig(config)
	c.Host = strings.TrimSuffix(c.Host, "/") + "/clusters/*"

	opts.NewInformerFunc = NewInformerWithClusterIndexes
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
	if opts.HTTPClient == nil {
		httpClient, err := NewClusterAwareHTTPClient(config)
		if err != nil {
			return nil, err
		}
		opts.HTTPClient = httpClient
	}
	if opts.Mapper == nil && opts.MapperWithContext == nil {
		opts.MapperWithContext = NewClusterAwareMapperProvider(config, opts.HTTPClient)
	}
	return client.NewAPIReader(config, opts)
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
func NewClusterAwareClient(config *rest.Config, opts client.Options) (client.Client, error) {
	if opts.HTTPClient == nil {
		httpClient, err := NewClusterAwareHTTPClient(config)
		if err != nil {
			return nil, err
		}
		opts.HTTPClient = httpClient
	}
	if opts.Mapper == nil && opts.MapperWithContext == nil {
		opts.MapperWithContext = NewClusterAwareMapperProvider(config, opts.HTTPClient)
	}
	return client.New(config, opts)
}

// NewClusterAwareHTTPClient returns an http.Client with a cluster aware round tripper.
func NewClusterAwareHTTPClient(config *rest.Config) (*http.Client, error) {
	httpClient, err := rest.HTTPClientFor(config)
	if err != nil {
		return nil, err
	}

	httpClient.Transport = newClusterAwareRoundTripper(httpClient.Transport)
	return httpClient, nil
}

// NewClusterAwareMapperProvider returns a function producing RESTMapper for the
// cluster specified in the context.
func NewClusterAwareMapperProvider(c *rest.Config, httpClient *http.Client) func(ctx context.Context) (meta.RESTMapper, error) {
	return func(ctx context.Context) (meta.RESTMapper, error) {
		cluster, _ := kontext.ClusterFrom(ctx) // intentionally ignoring second "found" return value
		cl := *httpClient
		cl.Transport = clusterRoundTripper{cluster: cluster.Path(), delegate: httpClient.Transport}
		return apiutil.NewDynamicRESTMapper(c, &cl)
	}
}

// newWildcardClusterMapperProvider returns a RESTMapper that talks to the /clusters/* endpoint.
func newWildcardClusterMapperProvider(c *rest.Config, httpClient *http.Client) (meta.RESTMapper, error) {
	mapperCfg := rest.CopyConfig(c)
	if !strings.HasSuffix(mapperCfg.Host, "/clusters/*") {
		mapperCfg.Host += "/clusters/*"
	}

	return apiutil.NewDynamicRESTMapper(mapperCfg, httpClient)
}

// ClusterAwareBuilderWithOptions returns a cluster aware Cache constructor that will build
// a cache honoring the options argument, this is useful to specify options like
// SelectorsDefaultNamespaces
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
		if options.SyncPeriod == nil {
			options.SyncPeriod = opts.SyncPeriod
		}
		if opts.DefaultNamespaces == nil {
			opts.DefaultNamespaces = options.DefaultNamespaces
		}

		return NewClusterAwareCache(config, options)
	}
}

// clusterAwareRoundTripper is a cluster-aware wrapper around http.RoundTripper
// taking the cluster from the context.
type clusterAwareRoundTripper struct {
	delegate http.RoundTripper
}

// newClusterAwareRoundTripper creates a new cluster aware round tripper.
func newClusterAwareRoundTripper(delegate http.RoundTripper) *clusterAwareRoundTripper {
	return &clusterAwareRoundTripper{
		delegate: delegate,
	}
}

func (c *clusterAwareRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	cluster, ok := kontext.ClusterFrom(req.Context())
	if ok && !cluster.Empty() {
		return clusterRoundTripper{cluster: cluster.Path(), delegate: c.delegate}.RoundTrip(req)
	}
	return c.delegate.RoundTrip(req)
}

// clusterRoundTripper is static cluster-aware wrapper around http.RoundTripper.
type clusterRoundTripper struct {
	cluster  logicalcluster.Path
	delegate http.RoundTripper
}

func (c clusterRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if !c.cluster.Empty() {
		req = req.Clone(req.Context())
		req.URL.Path = generatePath(req.URL.Path, c.cluster)
		req.URL.RawPath = generatePath(req.URL.RawPath, c.cluster)
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
